// Copyright 2016 Documize Inc. <legal@documize.com>. All rights reserved.
//
// This software (Documize Community Edition) is licensed under
// GNU AGPL v3 http://www.gnu.org/licenses/agpl-3.0.en.html
//
// You can operate outside the AGPL restrictions by purchasing
// Documize Enterprise Edition and obtaining a commercial license
// by contacting <sales@documize.com>.
//
// https://documize.com

package database

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/documize/community/core/env"
	"github.com/documize/community/core/streamutil"
	"github.com/documize/community/server/web"
)

var dbCheckOK bool // default false

// Check that the database is configured correctly and that all the required tables exist.
// It must be the first function called in this package.
func Check(runtime *env.Runtime) bool {
	runtime.Log.Info("Database checks: started")

	csBits := strings.Split(runtime.Flags.DBConn, "/")
	if len(csBits) > 1 {
		web.SiteInfo.DBname = strings.Split(csBits[len(csBits)-1], "?")[0]
	}

	rows, err := runtime.Db.Query("SELECT VERSION() AS version, @@version_comment as comment, @@character_set_database AS charset, @@collation_database AS collation")
	if err != nil {
		runtime.Log.Error("Can't get MySQL configuration", err)
		web.SiteInfo.Issue = "Can't get MySQL configuration: " + err.Error()
		runtime.Flags.SiteMode = env.SiteModeBadDB
		return false
	}
	defer streamutil.Close(rows)
	var version, dbComment, charset, collation string
	if rows.Next() {
		err = rows.Scan(&version, &dbComment, &charset, &collation)
	}

	if err == nil {
		err = rows.Err() // get any error encountered during iteration
	}

	if err != nil {
		runtime.Log.Error("no MySQL configuration returned", err)
		web.SiteInfo.Issue = "no MySQL configuration return issue: " + err.Error()
		runtime.Flags.SiteMode = env.SiteModeBadDB
		return false
	}

	// Get SQL variant as this affects minimum version checking logic.
	// MySQL and Percona share same version scheme (e..g 5.7.10).
	// MariaDB starts at 10.2.x
	runtime.DbVariant = GetSQLVariant(runtime.Flags.DBType, dbComment)
	runtime.Log.Info(fmt.Sprintf("Database checks: SQL variant %v", runtime.DbVariant))
	runtime.Log.Info("Database checks: SQL version " + version)

	verNums, err := GetSQLVersion(version)
	if err != nil {
		runtime.Log.Error("Database version check failed", err)
	}

	// Check minimum MySQL version as we need JSON column type.
	verInts := []int{5, 7, 10} // Minimum MySQL version
	if runtime.DbVariant == env.DBVariantMariaDB {
		verInts = []int{10, 3, 0} // Minimum MariaDB version
	}

	for k, v := range verInts {
		// If major release is higher then skip minor/patch checks (e.g. 8.x.x > 5.x.x)
		if k == 0 && len(verNums) > 0 && verNums[0] > verInts[0] {
			break
		}
		if verNums[k] < v {
			want := fmt.Sprintf("%d.%d.%d", verInts[0], verInts[1], verInts[2])
			runtime.Log.Error("MySQL version element "+strconv.Itoa(k+1)+" of '"+version+"' not high enough, need at least version "+want, errors.New("bad MySQL version"))
			web.SiteInfo.Issue = "MySQL version element " + strconv.Itoa(k+1) + " of '" + version + "' not high enough, need at least version " + want
			runtime.Flags.SiteMode = env.SiteModeBadDB
			return false
		}
	}

	{ // check the MySQL character set and collation
		if charset != "utf8" && charset != "utf8mb4" {
			runtime.Log.Error("MySQL character set not utf8/utf8mb4:", errors.New(charset))
			web.SiteInfo.Issue = "MySQL character set not utf8/utf8mb4: " + charset
			runtime.Flags.SiteMode = env.SiteModeBadDB
			return false
		}
		if !strings.HasPrefix(collation, "utf8") {
			runtime.Log.Error("MySQL collation sequence not utf8...:", errors.New(collation))
			web.SiteInfo.Issue = "MySQL collation sequence not utf8...: " + collation
			runtime.Flags.SiteMode = env.SiteModeBadDB
			return false
		}
	}

	{ // if there are no rows in the database, enter set-up mode
		var flds []string
		if err := runtime.Db.Select(&flds,
			`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = '`+web.SiteInfo.DBname+
				`' and TABLE_TYPE='BASE TABLE'`); err != nil {
			runtime.Log.Error("Can't get MySQL number of tables", err)
			web.SiteInfo.Issue = "Can't get MySQL number of tables: " + err.Error()
			runtime.Flags.SiteMode = env.SiteModeBadDB
			return false
		}
		if strings.TrimSpace(flds[0]) == "0" {
			runtime.Log.Info("Entering database set-up mode because the database is empty.....")
			runtime.Flags.SiteMode = env.SiteModeSetup
			return false
		}
	}

	{ // check all the required tables exist
		var tables = []string{`account`,
			`attachment`, `document`,
			`label`, `organization`,
			`page`, `revision`, `search`, `user`}

		for _, table := range tables {
			var dummy []string
			if err := runtime.Db.Select(&dummy, "SELECT 1 FROM "+table+" LIMIT 1;"); err != nil {
				runtime.Log.Error("Entering bad database mode because: SELECT 1 FROM "+table+" LIMIT 1;", err)
				web.SiteInfo.Issue = "MySQL database is not empty, but does not contain table: " + table
				runtime.Flags.SiteMode = env.SiteModeBadDB
				return false
			}
		}
	}

	runtime.Flags.SiteMode = env.SiteModeNormal // actually no need to do this (as already ""), this for documentation
	web.SiteInfo.DBname = ""                    // do not give this info when not in set-up mode
	dbCheckOK = true
	return true
}

// GetSQLVariant uses database value form @@version_comment to deduce MySQL variant.
func GetSQLVariant(dbType, vc string) env.DbVariant {
	vc = strings.ToLower(vc)
	dbType = strings.ToLower(dbType)

	// determine type from database
	if strings.Contains(vc, "mariadb") {
		return env.DBVariantMariaDB
	} else if strings.Contains(vc, "percona") {
		return env.DBVariantPercona
	} else if strings.Contains(vc, "mysql") {
		return env.DbVariantMySQL
	}

	// now determine type from command line switch
	if strings.Contains(dbType, "mariadb") {
		return env.DBVariantMariaDB
	} else if strings.Contains(dbType, "percona") {
		return env.DBVariantPercona
	} else if strings.Contains(dbType, "mysql") {
		return env.DbVariantMySQL
	}

	// horrid default could cause app to crash
	return env.DbVariantMySQL
}

// GetSQLVersion returns SQL version as major,minor,patch numerics.
func GetSQLVersion(v string) (ints []int, err error) {
	ints = []int{0, 0, 0}

	pos := strings.Index(v, "-")
	if pos > 1 {
		v = v[:pos]
	}

	vs := strings.Split(v, ".")

	if len(vs) < 3 {
		err = errors.New("MySQL version not of the form a.b.c")
		return
	}

	for key, val := range vs {
		num, err := strconv.Atoi(val)

		if err != nil {
			return ints, err
		}

		ints[key] = num
	}

	return
}
