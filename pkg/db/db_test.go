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
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPostgreSQLConnectionString(t *testing.T) {
	t.Run("with schema", func(t *testing.T) {
		connStr := getPostgreSQLConnectionString("localhost:5432", "universaltasks", "secret", "universaltasks", "universaltasks", "disable", "", "", "", "")
		assert.Equal(t, "postgres://universaltasks:secret@localhost:5432/universaltasks?sslmode=disable&sslcert=&sslkey=&sslrootcert=&search_path=%22universaltasks%22%2Cpublic", connStr)
	})
	t.Run("without schema", func(t *testing.T) {
		connStr := getPostgreSQLConnectionString("localhost:5432", "universaltasks", "secret", "universaltasks", "", "disable", "", "", "", "")
		assert.Equal(t, "postgres://universaltasks:secret@localhost:5432/universaltasks?sslmode=disable&sslcert=&sslkey=&sslrootcert=", connStr)
	})
	t.Run("schema needing quoting", func(t *testing.T) {
		connStr := getPostgreSQLConnectionString("localhost:5432", "universaltasks", "secret", "universaltasks", "MySchema", "disable", "", "", "", "")
		assert.Equal(t, "postgres://universaltasks:secret@localhost:5432/universaltasks?sslmode=disable&sslcert=&sslkey=&sslrootcert=&search_path=%22MySchema%22%2Cpublic", connStr)
	})
	t.Run("query exec mode", func(t *testing.T) {
		connStr := getPostgreSQLConnectionString("localhost:5432", "universaltasks", "secret", "universaltasks", "", "disable", "", "", "", "exec")
		assert.Equal(t, "postgres://universaltasks:secret@localhost:5432/universaltasks?sslmode=disable&sslcert=&sslkey=&sslrootcert=&default_query_exec_mode=exec", connStr)
	})
	t.Run("unix socket", func(t *testing.T) {
		connStr := getPostgreSQLConnectionString("/var/run/postgresql", "universaltasks", "secret", "universaltasks", "public", "disable", "", "", "", "")
		assert.Equal(t, "postgres://universaltasks:secret@:5432/universaltasks?sslmode=disable&sslcert=&sslkey=&sslrootcert=&host=/var/run/postgresql&search_path=%22public%22", connStr)
	})
	t.Run("special characters in credentials", func(t *testing.T) {
		for _, host := range []string{"db:5432", "/var/run/postgresql"} {
			connStr := getPostgreSQLConnectionString(host, "vi:k@user", "aa@-Dx!:%40/?#+ &", "universaltasks", "", "disable", "", "", "", "")
			cfg, err := pgconn.ParseConfig(connStr)
			require.NoError(t, err)
			assert.Equal(t, "vi:k@user", cfg.User)
			assert.Equal(t, "aa@-Dx!:%40/?#+ &", cfg.Password)
			assert.Equal(t, "universaltasks", cfg.Database)
		}
	})
}

func TestSanitizePostgresConnectionError(t *testing.T) {
	err := errors.New(`parse "postgres://universaltasks:secret@invalid host/universaltasks": invalid IP-literal`)

	sanitized := sanitizePostgresConnectionError(err, "universaltasks", "secret")

	assert.NotContains(t, sanitized.Error(), "universaltasks:secret")
	assert.NotContains(t, sanitized.Error(), "secret")
	assert.Contains(t, sanitized.Error(), "postgres://<redacted>@")
	assert.Contains(t, sanitized.Error(), "invalid IP-literal")
}

func TestSanitizePostgresConnectionErrorWithShortPassword(t *testing.T) {
	err := errors.New(`parse "postgres://postgres:x@[object Object]:5432/railway": invalid IP-literal`)

	sanitized := sanitizePostgresConnectionError(err, "postgres", "x")

	assert.NotContains(t, sanitized.Error(), "postgres:x@")
	assert.Contains(t, sanitized.Error(), "postgres://<redacted>@")
	assert.Contains(t, sanitized.Error(), "invalid IP-literal")
}
