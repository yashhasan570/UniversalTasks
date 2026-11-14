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

package db

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"code.universaltasks.io/api/pkg/config"
	"code.universaltasks.io/api/pkg/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	log.InitLogger()
	os.Exit(m.Run())
}

func Test_resolveDatabasePath(t *testing.T) {
	mockUserDataDir := func(path string) func() (string, error) {
		return func() (string, error) {
			return path, nil
		}
	}

	mockUserDataDirError := func() (string, error) {
		return "", fmt.Errorf("no home directory")
	}

	tests := []struct {
		name        string
		cfg         DatabasePathConfig
		userDataDir func() (string, error)
		expected    string
		expectError bool
	}{
		{
			name: "memory database",
			cfg: DatabasePathConfig{
				ConfiguredPath: "memory",
				RootPath:       "/opt/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "memory",
		},

		{
			name: "absolute path should be used as-is",
			cfg: DatabasePathConfig{
				ConfiguredPath: "/var/lib/universaltasks/universaltasks.db",
				RootPath:       "/opt/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "/var/lib/universaltasks/universaltasks.db",
		},
		{
			name: "absolute path with different rootpath still used as-is",
			cfg: DatabasePathConfig{
				ConfiguredPath: "/data/mydb.db",
				RootPath:       "/custom/path",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "/data/mydb.db",
		},

		{
			name: "relative path with explicit rootpath",
			cfg: DatabasePathConfig{
				ConfiguredPath: "universaltasks.db",
				RootPath:       "/var/lib/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "/var/lib/universaltasks/universaltasks.db",
		},
		{
			name: "relative subdirectory path with explicit rootpath",
			cfg: DatabasePathConfig{
				ConfiguredPath: "data/universaltasks.db",
				RootPath:       "/var/lib/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "/var/lib/universaltasks/data/universaltasks.db",
		},

		{
			name: "relative path with default rootpath uses user data dir",
			cfg: DatabasePathConfig{
				ConfiguredPath: "universaltasks.db",
				RootPath:       "/opt/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "/home/user/.local/share/universaltasks/universaltasks.db",
		},

		{
			name: "os.Executable failure falls back to user data dir",
			cfg: DatabasePathConfig{
				ConfiguredPath: "universaltasks.db",
				RootPath:       "/opt/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "/home/user/.local/share/universaltasks/universaltasks.db",
		},

		{
			name: "falls back to rootpath when userDataDir fails",
			cfg: DatabasePathConfig{
				ConfiguredPath: "universaltasks.db",
				RootPath:       "/opt/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDirError,
			expected:    "/opt/universaltasks/universaltasks.db",
		},

		{
			name: "empty configured path with explicit rootpath",
			cfg: DatabasePathConfig{
				ConfiguredPath: "",
				RootPath:       "/var/lib/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "/var/lib/universaltasks",
		},
		{
			name: "empty configured path with default rootpath",
			cfg: DatabasePathConfig{
				ConfiguredPath: "",
				RootPath:       "/opt/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "/home/user/.local/share/universaltasks",
		},
		{
			name: "path with dots normalized",
			cfg: DatabasePathConfig{
				ConfiguredPath: "/var/lib/universaltasks/../universaltasks/./db.db",
				RootPath:       "/opt/universaltasks",
				ExecutablePath: "/opt/universaltasks",
			},
			userDataDir: mockUserDataDir("/home/user/.local/share/universaltasks"),
			expected:    "/var/lib/universaltasks/db.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveDatabasePath(tt.cfg, tt.userDataDir)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_resolveDatabasePath_Integration(t *testing.T) {
	t.Run("with explicitly configured rootpath", func(t *testing.T) {
		cfg := DatabasePathConfig{
			ConfiguredPath: "universaltasks.db",
			RootPath:       "/custom/path",
			ExecutablePath: "/opt/universaltasks",
		}

		result, err := resolveDatabasePath(cfg, resolveUserDataDir)
		require.NoError(t, err)

		expected := filepath.Join("/custom/path", "universaltasks.db")
		assert.Equal(t, expected, result)
	})

	t.Run("with default rootpath uses user data directory", func(t *testing.T) {
		execPath, err := os.Executable()
		require.NoError(t, err)
		execDir := filepath.Dir(execPath)

		cfg := DatabasePathConfig{
			ConfiguredPath: "universaltasks.db",
			RootPath:       execDir,
			ExecutablePath: execDir,
		}

		result, err := resolveDatabasePath(cfg, resolveUserDataDir)
		require.NoError(t, err)

		assert.NotEqual(t, filepath.Join(execDir, "universaltasks.db"), result)
		assert.Contains(t, result, "universaltasks.db")

		switch runtime.GOOS {
		case "windows":
			assert.Contains(t, result, "Universal Tasks")
		case "darwin":
			assert.Contains(t, result, "Library")
			assert.Contains(t, result, "Application Support")
		default:
			assert.NotEqual(t,
				filepath.Dir(result),
				execDir,
				"Database should not be in executable directory",
			)
		}
	})

	t.Run("with subdirectory path", func(t *testing.T) {
		cfg := DatabasePathConfig{
			ConfiguredPath: "data/universaltasks.db",
			RootPath:       "/custom/path",
			ExecutablePath: "/opt/universaltasks",
		}

		result, err := resolveDatabasePath(cfg, resolveUserDataDir)
		require.NoError(t, err)

		expected := filepath.Join("/custom/path", "data", "universaltasks.db")
		assert.Equal(t, expected, result)
	})
}

func Test_resolveDatabasePath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on non-Windows platform")
	}

	mockUserDataDir := func(path string) func() (string, error) {
		return func() (string, error) {
			return path, nil
		}
	}

	tests := []struct {
		name        string
		cfg         DatabasePathConfig
		userDataDir func() (string, error)
		expected    string
	}{
		{
			name: "windows absolute path",
			cfg: DatabasePathConfig{
				ConfiguredPath: "C:\\ProgramData\\Universal Tasks\\universaltasks.db",
				RootPath:       "C:\\Program Files\\Universal Tasks",
				ExecutablePath: "C:\\Program Files\\Universal Tasks",
			},
			userDataDir: mockUserDataDir("C:\\Users\\test\\AppData\\Local\\Universal Tasks"),
			expected:    "C:\\ProgramData\\Universal Tasks\\universaltasks.db",
		},
		{
			name: "windows relative path with explicit rootpath",
			cfg: DatabasePathConfig{
				ConfiguredPath: "universaltasks.db",
				RootPath:       "C:\\ProgramData\\Universal Tasks",
				ExecutablePath: "C:\\Program Files\\Universal Tasks",
			},
			userDataDir: mockUserDataDir("C:\\Users\\test\\AppData\\Local\\Universal Tasks"),
			expected:    "C:\\ProgramData\\Universal Tasks\\universaltasks.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveDatabasePath(tt.cfg, tt.userDataDir)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveUserDataDir(t *testing.T) {
	test := func() string {
		dataDir, err := resolveUserDataDir()
		require.NoError(t, err)
		assert.NotEmpty(t, dataDir)

		_, err = os.Stat(dataDir)
		require.ErrorIs(t, err, os.ErrNotExist, "resolveUserDataDir must not create the directory")

		return dataDir
	}

	// Verify platform-specific paths. Env vars point at fresh t.TempDir()s so the
	// "not created" assertion isn't confused by a directory left over from real usage.
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "AppData", "Local"))
		dataDir := test()
		assert.Contains(t, dataDir, "Universal Tasks")
	case "darwin":
		t.Setenv("HOME", t.TempDir())
		dataDir := test()
		assert.Contains(t, dataDir, "Library")
		assert.Contains(t, dataDir, "Application Support")
		assert.Contains(t, dataDir, "Universal Tasks")
	default:
		t.Run("with XDG_DATA_HOME", func(t *testing.T) {
			xdgDataHome := t.TempDir()
			t.Setenv("XDG_DATA_HOME", xdgDataHome)
			dataDir := test()
			assert.Equal(t, filepath.Join(xdgDataHome, "universaltasks"), dataDir)
		})

		t.Run("without XDG_DATA_HOME", func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", "")
			home := t.TempDir()
			t.Setenv("HOME", home)
			dataDir := test()
			assert.Equal(t, filepath.Join(home, ".local", "share", "universaltasks"), dataDir)
		})
	}
}

func setUserDataDirForTest(t *testing.T, basePath string) string {
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", basePath)
		return filepath.Join(basePath, "Universal Tasks")
	case "darwin":
		t.Setenv("HOME", basePath)
		return filepath.Join(basePath, "Library", "Application Support", "Universal Tasks")
	default:
		t.Setenv("XDG_DATA_HOME", basePath)
		return filepath.Join(basePath, "universaltasks")
	}
}

func setupDatabasePathTest(t *testing.T, configuredPath string) (dataDir string) {
	t.Cleanup(config.ResetForTests)

	dataDir = setUserDataDirForTest(t, filepath.Join(t.TempDir(), "data"))

	execPath, err := os.Executable()
	require.NoError(t, err)

	config.DatabasePath.Set(configuredPath)
	// Matching the executable dir is what makes resolution use the data dir.
	config.ServiceRootpath.Set(filepath.Dir(execPath))

	return dataDir
}

func TestResolvedDatabasePath_DoesNotCreateDataDir(t *testing.T) {
	dataDir := setupDatabasePathTest(t, "universaltasks.db")

	path, err := ResolvedDatabasePath()
	require.NoError(t, err)
	assert.Contains(t, path, "universaltasks.db")

	_, err = os.Stat(dataDir)
	require.ErrorIs(t, err, os.ErrNotExist, "ResolvedDatabasePath must not create the data directory")
}

func TestResolvedDatabasePath_MatchesEnsureDatabasePath(t *testing.T) {
	dataDir := setupDatabasePathTest(t, "universaltasks.db")

	// Order matters: until the data directory exists, resolution reports the rootpath
	// fallback instead - see TestResolvedDatabasePath_FallsBackWhileDataDirMissing.
	ensured, err := ensureDatabasePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dataDir, "universaltasks.db"), ensured)

	info, err := os.Stat(filepath.Dir(ensured))
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "ensureDatabasePath must create the directory it returns")

	resolved, err := ResolvedDatabasePath()
	require.NoError(t, err)

	assert.Equal(t, ensured, resolved)
}

// TestResolvedDatabasePath_FallsBackWhileDataDirMissing pins the one case where doctor
// and the server disagree: a data directory nobody created yet.
func TestResolvedDatabasePath_FallsBackWhileDataDirMissing(t *testing.T) {
	setupDatabasePathTest(t, "universaltasks.db")

	resolved, err := ResolvedDatabasePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(config.ServiceRootpath.GetString(), "universaltasks.db"), resolved)
}

func TestEnsureDatabasePath_DoesNotCreateConfiguredDirectories(t *testing.T) {
	t.Run("absolute path", func(t *testing.T) {
		missingParent := filepath.Join(t.TempDir(), "deep", "nested")
		setupDatabasePathTest(t, filepath.Join(missingParent, "universaltasks.db"))

		path, err := ensureDatabasePath()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(missingParent, "universaltasks.db"), path)

		_, err = os.Stat(missingParent)
		require.ErrorIs(t, err, os.ErrNotExist, "ensureDatabasePath must not create a configured path's parents")
	})

	t.Run("rootpath relative path", func(t *testing.T) {
		setupDatabasePathTest(t, filepath.Join("sub", "dir", "universaltasks.db"))
		rootPath := filepath.Join(t.TempDir(), "install")
		config.ServiceRootpath.Set(rootPath)

		path, err := ensureDatabasePath()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(rootPath, "sub", "dir", "universaltasks.db"), path)

		_, err = os.Stat(rootPath)
		require.ErrorIs(t, err, os.ErrNotExist, "ensureDatabasePath must not create the rootpath")
	})
}

func TestEnsureDatabasePath_CreatesUserDataDir(t *testing.T) {
	dataDir := setupDatabasePathTest(t, "universaltasks.db")

	path, err := ensureDatabasePath()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dataDir, "universaltasks.db"), path)

	info, err := os.Stat(dataDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureDatabasePath_FallsBackWhenDataDirIsNotCreatable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("directory permissions do not apply here")
	}

	setupDatabasePathTest(t, "universaltasks.db")

	readOnly := t.TempDir()
	setUserDataDirForTest(t, filepath.Join(readOnly, "data"))
	require.NoError(t, os.Chmod(readOnly, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })

	path, err := ensureDatabasePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(config.ServiceRootpath.GetString(), "universaltasks.db"), path)

	resolved, err := ResolvedDatabasePath()
	require.NoError(t, err)
	assert.Equal(t, path, resolved, "doctor must report the path the server falls back to")
}

func TestIsSystemDirectory(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Windows system directories
		{
			name:     "Windows System32",
			path:     "C:\\Windows\\System32\\universaltasks.db",
			expected: runtime.GOOS == "windows",
		},
		{
			name:     "Windows SysWOW64",
			path:     "C:\\Windows\\SysWOW64\\universaltasks.db",
			expected: runtime.GOOS == "windows",
		},
		{
			name:     "Windows root",
			path:     "C:\\Windows\\universaltasks.db",
			expected: runtime.GOOS == "windows",
		},
		{
			name:     "Windows System32 lowercase",
			path:     "c:\\windows\\system32\\universaltasks.db",
			expected: runtime.GOOS == "windows",
		},
		// Unix-like system directories
		{
			name:     "/bin",
			path:     "/bin/universaltasks.db",
			expected: runtime.GOOS != "windows",
		},
		{
			name:     "/sbin",
			path:     "/sbin/universaltasks.db",
			expected: runtime.GOOS != "windows",
		},
		{
			name:     "/usr/bin",
			path:     "/usr/bin/universaltasks.db",
			expected: runtime.GOOS != "windows",
		},
		{
			name:     "/etc",
			path:     "/etc/universaltasks.db",
			expected: runtime.GOOS != "windows",
		},
		// Non-system directories
		{
			name:     "user home directory (Unix)",
			path:     "/home/user/universaltasks.db",
			expected: false,
		},
		{
			name:     "user profile directory (Windows)",
			path:     "C:\\Users\\user\\universaltasks.db",
			expected: false,
		},
		{
			name:     "custom directory",
			path:     "/opt/universaltasks/universaltasks.db",
			expected: false,
		},
		{
			name:     "relative path",
			path:     "./universaltasks.db",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSystemDirectory(tt.path)
			assert.Equal(t, tt.expected, result,
				"Expected isSystemDirectory(%s) to be %v on %s",
				tt.path, tt.expected, runtime.GOOS)
		})
	}
}

func TestIsSystemDirectory_EdgeCases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Run("false positives - paths containing 'windows' but not system directories", func(t *testing.T) {
			tests := []struct {
				name string
				path string
			}{
				{
					name: "custom app with windows in path",
					path: "C:\\myapp\\windows\\data\\universaltasks.db",
				},
				{
					name: "windows directory on non-C drive",
					path: "D:\\windows\\universaltasks.db",
				},
				{
					name: "user directory named windows",
					path: "C:\\Users\\windows\\universaltasks.db",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					assert.False(t, isSystemDirectory(tt.path))
				})
			}
		})

		t.Run("safe Windows subdirectories", func(t *testing.T) {
			assert.False(t, isSystemDirectory("C:\\Windows\\Temp\\universaltasks.db"))
		})

		t.Run("actual Windows system directories", func(t *testing.T) {
			tests := []struct {
				name string
				path string
			}{
				{
					name: "Windows root",
					path: "C:\\Windows\\universaltasks.db",
				},
				{
					name: "Windows root lowercase",
					path: "c:\\windows\\universaltasks.db",
				},
				{
					name: "System32",
					path: "C:\\Windows\\System32\\universaltasks.db",
				},
				{
					name: "System32 uppercase",
					path: "C:\\WINDOWS\\SYSTEM32\\universaltasks.db",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					assert.True(t, isSystemDirectory(tt.path))
				})
			}
		})
	} else {
		t.Run("false positives - paths containing system dir names", func(t *testing.T) {
			tests := []struct {
				name string
				path string
			}{
				{
					name: "/home/bin not same as /bin",
					path: "/home/bin/universaltasks.db",
				},
				{
					name: "/opt/sbin not same as /sbin",
					path: "/opt/sbin/universaltasks.db",
				},
				{
					name: "/usr/local/bin is safe",
					path: "/usr/local/bin/universaltasks.db",
				},
				{
					name: "/binaries not same as /bin",
					path: "/binaries/universaltasks.db",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					assert.False(t, isSystemDirectory(tt.path))
				})
			}
		})

		t.Run("actual Unix system directories", func(t *testing.T) {
			tests := []struct {
				name string
				path string
			}{
				{
					name: "/bin",
					path: "/bin/universaltasks.db",
				},
				{
					name: "/etc",
					path: "/etc/universaltasks.db",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					assert.True(t, isSystemDirectory(tt.path))
				})
			}
		})
	}
}
