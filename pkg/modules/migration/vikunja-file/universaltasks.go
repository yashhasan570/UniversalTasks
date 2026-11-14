// Universal Tasks is a to-do list application to facilitate your life.
// Copyright 2018-present Universal Tasks and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package universaltasksfile

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"code.universaltasks.io/api/pkg/config"
	"code.universaltasks.io/api/pkg/db"
	"code.universaltasks.io/api/pkg/events"
	"code.universaltasks.io/api/pkg/files"
	"code.universaltasks.io/api/pkg/log"
	"code.universaltasks.io/api/pkg/models"
	"code.universaltasks.io/api/pkg/modules/migration"
	"code.universaltasks.io/api/pkg/user"
	"code.universaltasks.io/api/pkg/utils"
	vversion "code.universaltasks.io/api/pkg/version"
	"code.universaltasks.io/api/pkg/web"

	"github.com/c2h5oh/datasize"
	"github.com/hashicorp/go-version"
	"xorm.io/builder"
)

const logPrefix = "[Universal Tasks File Import] "

// minZipEntryCap ensures data.json / filters.json / VERSION entries can
// still be read when files.maxsize is tiny.
const minZipEntryCap = 4 * 1024 * 1024 // 4 MB

// maxZipEntrySize returns max(files.maxsize, minZipEntryCap).
func maxZipEntrySize() int64 {
	mb := config.GetMaxFileSizeInMBytes()
	// Clamp before multiply: absurd configs would overflow int64.
	const maxInt64MB = uint64(math.MaxInt64 / datasize.MB)
	if mb > maxInt64MB {
		return math.MaxInt64
	}
	fromConfig := int64(mb) * int64(datasize.MB)
	if fromConfig < minZipEntryCap {
		return minZipEntryCap
	}
	return fromConfig
}

// ErrFileTooLarge is returned when a file in the zip archive exceeds the effective cap.
var ErrFileTooLarge = fmt.Errorf("zip entry exceeds the configured maximum file size")

// readZipEntry detects cap overflow and charges actual bytes to the import budget.
func readZipEntry(r io.Reader, budget *importBudget) (*bytes.Buffer, error) {
	limit := maxZipEntrySize()
	// Avoid limit+1 overflowing to MinInt64 when limit == MaxInt64,
	// which io.LimitReader would treat as EOF (every entry reads empty).
	readCap := limit
	if readCap != math.MaxInt64 {
		readCap++
	}
	limitedReader := io.LimitReader(r, readCap)
	var buf bytes.Buffer
	n, err := buf.ReadFrom(limitedReader)
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, ErrFileTooLarge
	}
	if err := budget.count(n); err != nil {
		return nil, err
	}
	return &buf, nil
}

// importBudget verifies actual bytes after the ZIP metadata preflight (GHSA-w7jp-mf2v-8342).
type importBudget struct {
	remaining int64
}

type storageBudget struct {
	remaining int64
}

// ErrUniversalTasksFileImportTooLarge is returned when the export exceeds the
// configured size, file-count or storage-quota limits.
type ErrUniversalTasksFileImportTooLarge struct {
	Reason string
}

func (err *ErrUniversalTasksFileImportTooLarge) Error() string {
	return "The Universal Tasks export is too large: " + err.Reason
}

// ErrCodeUniversalTasksFileImportTooLarge holds the unique world-error code of this error
const ErrCodeUniversalTasksFileImportTooLarge = 14007

// HTTPError holds the http error description
func (err *ErrUniversalTasksFileImportTooLarge) HTTPError() web.HTTPError {
	return web.HTTPError{
		HTTPCode: http.StatusBadRequest,
		Code:     ErrCodeUniversalTasksFileImportTooLarge,
		Message:  "The Universal Tasks export is too large: " + err.Reason,
	}
}

func universaltasksFileMaxSize() (int64, error) {
	var size datasize.ByteSize
	if err := size.UnmarshalText([]byte(config.MigrationUniversalTasksFileMaxSize.GetString())); err != nil {
		return 0, fmt.Errorf("could not parse migration.universaltasksfile.maxsize: %w", err)
	}
	return int64(size.Bytes()), nil //nolint:gosec // config value is bounded in practice
}

func universaltasksFileMaxUserStorage() (int64, error) {
	var size datasize.ByteSize
	if err := size.UnmarshalText([]byte(config.MigrationUniversalTasksFileMaxUserStorage.GetString())); err != nil {
		return 0, fmt.Errorf("could not parse migration.universaltasksfile.maxuserstorage: %w", err)
	}
	return int64(size.Bytes()), nil //nolint:gosec // config value is bounded in practice
}

func (b *importBudget) count(n int64) error {
	b.remaining -= n
	if b.remaining < 0 {
		return &ErrUniversalTasksFileImportTooLarge{Reason: "it contains more decompressed data than migration.universaltasksfile.maxsize allows"}
	}
	return nil
}

func (b *storageBudget) count(n int64) error {
	if n > b.remaining {
		return &ErrUniversalTasksFileImportTooLarge{Reason: "it would exceed the import storage quota of migration.universaltasksfile.maxuserstorage"}
	}
	b.remaining -= n
	return nil
}

// lazyFileProvider holds only one decompressed ZIP entry at a time.
type lazyFileProvider struct {
	attachments map[*models.TaskAttachment]*zip.File
	backgrounds map[*models.ProjectWithTasksAndBuckets]*zip.File
	budget      *importBudget
	storage     *storageBudget
}

func (p *lazyFileProvider) OpenAttachment(attachment *models.TaskAttachment) (io.ReadSeekCloser, int64, error) {
	f, has := p.attachments[attachment]
	if !has {
		return nil, 0, nil
	}
	return p.openZipFile(f, true)
}

func (p *lazyFileProvider) OpenBackground(project *models.ProjectWithTasksAndBuckets) (io.ReadSeekCloser, int64, error) {
	f, has := p.backgrounds[project]
	if !has {
		return nil, 0, nil
	}
	return p.openZipFile(f, false)
}

func (p *lazyFileProvider) CountBackgroundFile(size int64) error {
	return p.storage.count(size)
}

// Oversized entries use files.ErrFileIsTooLarge to preserve skip behavior.
func (p *lazyFileProvider) openZipFile(f *zip.File, countStorage bool) (io.ReadSeekCloser, int64, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rc.Close() }()

	tmp, err := os.CreateTemp("", "universaltasks-import-*")
	if err != nil {
		return nil, 0, err
	}

	written, err := p.copyCounted(tmp, rc)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, err
	}
	if written > maxZipEntrySize() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, files.ErrFileIsTooLarge{Size: uint64(written)} //nolint:gosec // written is bounded by the budget
	}
	if countStorage {
		err = p.storage.count(written)
	}
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, err
	}
	return &tempFileReadSeekCloser{f: tmp}, written, nil
}

func (p *lazyFileProvider) copyCounted(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 64*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			total += int64(n)
			if berr := p.budget.count(int64(n)); berr != nil {
				return total, berr
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, werr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
	}
}

type tempFileReadSeekCloser struct {
	f *os.File
}

func (t *tempFileReadSeekCloser) Read(p []byte) (int, error) { return t.f.Read(p) }
func (t *tempFileReadSeekCloser) Seek(o int64, whence int) (int64, error) {
	return t.f.Seek(o, whence)
}
func (t *tempFileReadSeekCloser) Close() error {
	err := t.f.Close()
	_ = os.Remove(t.f.Name())
	return err
}

type FileMigrator struct {
}

// Name is used to get the name of the universaltasks-file migration - we're using the docs here to annotate the status route.
// @Summary Get migration status
// @Description Returns if the current user already did the migation or not. This is useful to show a confirmation message in the frontend if the user is trying to do the same migration again.
// @tags migration
// @Produce json
// @Security JWTKeyAuth
// @Success 200 {object} migration.Status "The migration status"
// @Failure 500 {object} models.Message "Internal server error"
// @Router /migration/universaltasks-file/status [get]
func (v *FileMigrator) Name() string {
	return "universaltasks-file"
}

// archive is the index of an export zip, built once its structural checks pass.
type archive struct {
	data        *zip.File
	filters     *zip.File
	version     *zip.File
	storedFiles map[int64]*zip.File
	maxSize     int64
}

func openArchive(file io.ReaderAt, size int64) (*zip.Reader, error) {
	r, err := zip.NewReader(file, size)
	if err != nil {
		if err.Error() == "zip: not a valid zip file" {
			return nil, &migration.ErrNotAZipFile{}
		}
		return nil, fmt.Errorf("could not open import file: %w", err)
	}
	return r, nil
}

// scanArchive indexes the entries and runs every check that does not need the
// entry contents, so a broken export is rejected before anything is imported.
func scanArchive(r *zip.Reader) (*archive, error) {
	log.Debugf(logPrefix+"Importing a zip file containing %d files", len(r.File))

	// files/ entries plus data.json, filters.json and VERSION
	if int64(len(r.File)) > config.MigrationUniversalTasksFileMaxFiles.GetInt64()+3 {
		return nil, &ErrUniversalTasksFileImportTooLarge{Reason: "it contains too many entries for migration.universaltasksfile.maxfiles"}
	}

	a := &archive{storedFiles: make(map[int64]*zip.File)}
	var storedFileCount int64
	for _, f := range r.File {
		if utils.ContainsPathTraversal(f.Name) {
			return nil, &migration.ErrInvalidImportFile{Err: fmt.Errorf("unsafe path in zip archive: %q", f.Name)}
		}

		if strings.HasPrefix(f.Name, "files/") {
			storedFileCount++
			if storedFileCount > config.MigrationUniversalTasksFileMaxFiles.GetInt64() {
				return nil, &ErrUniversalTasksFileImportTooLarge{Reason: "it contains more files than migration.universaltasksfile.maxfiles allows"}
			}
			fname := strings.TrimPrefix(f.Name, "files/")
			id, err := strconv.ParseInt(fname, 10, 64)
			if err != nil {
				return nil, &migration.ErrInvalidImportFile{Err: fmt.Errorf("could not convert file id: %w", err)}
			}
			if _, exists := a.storedFiles[id]; exists {
				return nil, &migration.ErrInvalidImportFile{Err: fmt.Errorf("duplicate file id %d", id)}
			}
			a.storedFiles[id] = f
			log.Debugf(logPrefix + "Found a blob file")
			continue
		}
		if f.Name == "data.json" {
			a.data = f
			log.Debugf(logPrefix + "Found a data file")
			continue
		}
		if f.Name == "filters.json" {
			a.filters = f
			log.Debugf(logPrefix + "Found a filter file")
		}
		if f.Name == "VERSION" {
			a.version = f
			log.Debugf(logPrefix + "Found a version file")
		}
	}

	if a.data == nil {
		return nil, &migration.ErrNoDataFileInZip{}
	}
	if a.version == nil {
		return nil, &migration.ErrInvalidImportFile{Err: errors.New("dump does not seem to contain a version file")}
	}

	// Preflight: bound the import before anything is read (GHSA-w7jp-mf2v-8342).
	maxSize, err := universaltasksFileMaxSize()
	if err != nil {
		return nil, err
	}
	a.maxSize = maxSize
	var totalUncompressed uint64
	for _, f := range r.File {
		totalUncompressed += f.UncompressedSize64
		if totalUncompressed < f.UncompressedSize64 {
			return nil, &ErrUniversalTasksFileImportTooLarge{Reason: "the sum of file sizes overflows"}
		}
	}
	if totalUncompressed > uint64(maxSize) { //nolint:gosec // maxSize fits uint64 by construction
		return nil, &ErrUniversalTasksFileImportTooLarge{Reason: "it decompresses to more than migration.universaltasksfile.maxsize allows"}
	}

	return a, nil
}

// ValidateFile rejects an upload that is not a usable export before the import
// is queued, so picking the wrong file still fails the request.
func (v *FileMigrator) ValidateFile(file io.ReaderAt, size int64) error {
	r, err := openArchive(file, size)
	if err != nil {
		return err
	}
	_, err = scanArchive(r)
	return err
}

// Migrate takes a universaltasks file export, parses it and imports everything in it into Universal Tasks.
// @Summary Import all projects, tasks etc. from a Universal Tasks data export
// @Description Imports all projects, tasks, notes, reminders, subtasks and files from a Vikunjda data export into Universal Tasks.
// @tags migration
// @Accept x-www-form-urlencoded
// @Produce json
// @Security JWTKeyAuth
// @Param import formData string true "The Universal Tasks export zip file."
// @Success 200 {object} models.Message "A message telling you the migration was started."
// @Failure 500 {object} models.Message "Internal server error"
// @Router /migration/universaltasks-file/migrate [post]
func (v *FileMigrator) Migrate(user *user.User, file io.ReaderAt, size int64) error {
	r, err := openArchive(file, size)
	if err != nil {
		return err
	}

	a, err := scanArchive(r)
	if err != nil {
		return err
	}
	dataFile, filterFile, versionFile, storedFiles, maxSize := a.data, a.filters, a.version, a.storedFiles, a.maxSize

	maxUserStorage, err := universaltasksFileMaxUserStorage()
	if err != nil {
		return err
	}
	qs := db.NewSession()
	var existingStorage int64
	_, err = qs.
		Table("files").
		Select("COALESCE(SUM(size), 0)").
		Where("created_by_id = ?", user.ID).
		// Uploads waiting for their import job are not the user's data, including this import's own.
		And(builder.NotIn("id", builder.Select("upload_file_id").From((&migration.Status{}).TableName()).Where(builder.NotNull{"upload_file_id"}))).
		Get(&existingStorage)
	_ = qs.Close()
	if err != nil {
		return fmt.Errorf("could not check the storage quota: %w", err)
	}
	if existingStorage > maxUserStorage {
		return &ErrUniversalTasksFileImportTooLarge{Reason: "it would exceed the import storage quota of migration.universaltasksfile.maxuserstorage"}
	}

	budget := &importBudget{remaining: maxSize}

	//////
	// Check if we're able to import this dump
	vf, err := versionFile.Open()
	if err != nil {
		return fmt.Errorf("could not open version file: %w", err)
	}
	defer vf.Close()

	bufVersion, err := readZipEntry(vf, budget)
	if err != nil {
		return fmt.Errorf("could not read version file: %w", err)
	}

	versionString := bufVersion.String()
	if versionString == "dev" && vversion.Version == "dev" {
		log.Debugf(logPrefix + "Importing from dev version")
	} else {
		dumpedVersion, err := version.NewVersion(bufVersion.String())
		if err != nil {
			return &migration.ErrInvalidImportFile{Err: fmt.Errorf("could not parse the version of the export: %w", err)}
		}
		minVersion, err := version.NewVersion("0.20.1+61")
		if err != nil {
			return err
		}

		if dumpedVersion.LessThan(minVersion) {
			return &migration.ErrImportFromUnsupportedVersion{DumpVersion: dumpedVersion.String(), MinVersion: minVersion.String()}
		}
	}

	//////
	// Import the bulk of Universal Tasks data
	df, err := dataFile.Open()
	if err != nil {
		return fmt.Errorf("could not open data file: %w", err)
	}
	defer df.Close()

	bufData, err := readZipEntry(df, budget)
	if err != nil {
		return fmt.Errorf("could not read data file: %w", err)
	}

	if err := checkDataItemCount(bufData.Bytes()); err != nil {
		return err
	}
	projects := []*models.ProjectWithTasksAndBuckets{}
	if err := json.Unmarshal(bufData.Bytes(), &projects); err != nil {
		return &migration.ErrInvalidImportFile{Err: fmt.Errorf("could not read data: %w", err)}
	}

	var filters []*models.SavedFilter
	if filterFile != nil {
		ff, err := filterFile.Open()
		if err != nil {
			return fmt.Errorf("could not open filters file: %w", err)
		}
		defer ff.Close()

		bufFilter, err := readZipEntry(ff, budget)
		if err != nil {
			return fmt.Errorf("could not read filters file: %w", err)
		}

		if err := checkFilterItemCount(bufFilter.Bytes()); err != nil {
			return err
		}
		if err := json.Unmarshal(bufFilter.Bytes(), &filters); err != nil {
			return &migration.ErrInvalidImportFile{Err: fmt.Errorf("could not read filter data: %w", err)}
		}
	}

	provider := &lazyFileProvider{
		attachments: map[*models.TaskAttachment]*zip.File{},
		backgrounds: map[*models.ProjectWithTasksAndBuckets]*zip.File{},
		budget:      budget,
		storage:     &storageBudget{remaining: maxUserStorage - existingStorage},
	}
	for _, p := range projects {
		err = addDetailsToProjectAndChildren(p, storedFiles, provider)
		if err != nil {
			return err
		}
	}

	err = migration.InsertFromStructureWithFileProvider(projects, user, provider)
	if err != nil {
		return fmt.Errorf("could not insert data: %w", err)
	}

	if len(filters) == 0 {
		log.Debugf(logPrefix + "No filters to import")
		return nil
	}

	log.Debugf(logPrefix+"Importing %d saved filters", len(filters))

	s := db.NewSession()
	defer s.Close()

	for _, f := range filters {
		f.ID = 0
		err = f.Create(s, user)
		if err != nil {
			_ = s.Rollback()
			events.CleanupPending(s)
			return err
		}
	}

	if err := s.Commit(); err != nil {
		events.CleanupPending(s)
		return err
	}

	events.DispatchPending(context.Background(), s)
	return nil
}

func addDetailsToProjectAndChildren(p *models.ProjectWithTasksAndBuckets, storedFiles map[int64]*zip.File, provider *lazyFileProvider) (err error) {
	err = addDetailsToProject(p, storedFiles, provider)
	if err != nil {
		return err
	}

	for _, cp := range p.ChildProjects {
		err = addDetailsToProjectAndChildren(cp, storedFiles, provider)
		if err != nil {
			return
		}
	}

	return
}

func addDetailsToProject(l *models.ProjectWithTasksAndBuckets, storedFiles map[int64]*zip.File, provider *lazyFileProvider) (err error) {
	var backgroundFileID int64
	bginfo, is := l.BackgroundInformation.(map[string]interface{})
	if is {
		bgid, has := bginfo["id"]
		if has {
			bgidFloat, ok := bgid.(float64)
			if !ok {
				return &migration.ErrInvalidImportFile{Err: fmt.Errorf("invalid background file id type: expected number, got %T", bgid)}
			}
			backgroundFileID = int64(bgidFloat)
		}
	}
	// Preserve the map so the insert does not mistake it for preloaded bytes.
	if b, exists := storedFiles[backgroundFileID]; exists {
		provider.backgrounds[l] = b
	}

	for _, t := range l.Tasks {
		for _, label := range t.Labels {
			label.ID = 0
		}
		for _, comment := range t.Comments {
			comment.ID = 0
		}
		for _, attachment := range t.Attachments {
			attachmentFile, exists := storedFiles[attachment.File.ID]
			if !exists {
				log.Debugf(logPrefix+"Could not find attachment file %d for attachment %d", attachment.File.ID, attachment.ID)
				continue
			}

			provider.attachments[attachment] = attachmentFile
			attachment.File.ID = 0
		}
	}

	return
}
