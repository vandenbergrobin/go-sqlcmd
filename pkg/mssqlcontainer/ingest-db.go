package mssqlcontainer

import (
	"fmt"
	"github.com/microsoft/go-sqlcmd/internal/container"
	output2 "github.com/microsoft/go-sqlcmd/internal/output"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func DownloadAndRestoreDb(
	controller *container.Controller,
	containerId string,
	usingDatabaseUrl string,
	userName string,
	password string,
	query func(commandText string),
	output *output2.Output,
) {
	databaseName := parseDbName(usingDatabaseUrl)
	databaseUrl := extractUrl(usingDatabaseUrl)

	_, file := filepath.Split(databaseUrl)

	// Download file from URL into container
	output.Infof("Downloading %s", file)

	u, _ := url.Parse(usingDatabaseUrl)

	_, f := filepath.Split(u.Path)

	var temporaryFolder string
	if filepath.Ext(f) == ".bak" || filepath.Ext(f) == ".7z" {
		temporaryFolder = "/var/opt/mssql/backup"
	} else if filepath.Ext(f) == ".mdf" {
		temporaryFolder = "/var/opt/mssql/data"
	} else {
		panic("Unsupported file extension")
	}

	controller.DownloadFile(
		containerId,
		databaseUrl,
		temporaryFolder,
	)

	if filepath.Ext(f) == ".7z" {
		controller.RunCmdInContainer(containerId, []string{
			"wget",
			"-O",
			"/opt/7-zip/7-zip.tar",
			"https://7-zip.org/a/7z2201-linux-x64.tar.xz"})

		controller.RunCmdInContainer(containerId, []string{"tar", "xvf", "/tmp/7-zip.tar"})
		controller.RunCmdInContainer(containerId, []string{"mkdir", "/opt/7-zip"})
		controller.RunCmdInContainer(containerId, []string{"chmod", "u+x", "/opt/7-zip/7zz"})
		controller.RunCmdInContainer(containerId, []string{
			"/opt/7-zip/7zz",
			"x",
			"-o/var/opt/mssql/data",
			temporaryFolder + "/" + file,
		})

		controller.RunCmdInContainer(containerId, []string{
			"./opt/7-zip/7zz",
			"l",
			"-ba",
			"-slt",
			temporaryFolder + "/" + file,
		})

		databaseName = "StackOverflow2010"
		f = "StackOverflow2010.mdf"
		temporaryFolder = "/var/opt/mssql/data"
	}

	dbNameAsIdentifier := getDbNameAsIdentifier(databaseName)
	dbNameAsNonIdentifier := getDbNameAsNonIdentifier(databaseName)

	if filepath.Ext(f) == ".bak" {
		// Restore database from file
		output.Infof("Restoring database %s", databaseName)

		text := `SET NOCOUNT ON;

-- Build a SQL Statement to restore any .bak file to the Linux filesystem
DECLARE @sql NVARCHAR(max)

-- This table definition works since SQL Server 2017, therefore 
-- works for all SQL Server containers (which started in 2017)
DECLARE @fileListTable TABLE (
    [LogicalName]           NVARCHAR(128),
    [PhysicalName]          NVARCHAR(260),
    [Type]                  CHAR(1),
    [FileGroupName]         NVARCHAR(128),
    [Size]                  NUMERIC(20,0),
    [MaxSize]               NUMERIC(20,0),
    [FileID]                BIGINT,
    [CreateLSN]             NUMERIC(25,0),
    [DropLSN]               NUMERIC(25,0),
    [UniqueID]              UNIQUEIDENTIFIER,
    [ReadOnlyLSN]           NUMERIC(25,0),
    [ReadWriteLSN]          NUMERIC(25,0),
    [BackupSizeInBytes]     BIGINT,
    [SourceBlockSize]       INT,
    [FileGroupID]           INT,
    [LogGroupGUID]          UNIQUEIDENTIFIER,
    [DifferentialBaseLSN]   NUMERIC(25,0),
    [DifferentialBaseGUID]  UNIQUEIDENTIFIER,
    [IsReadOnly]            BIT,
    [IsPresent]             BIT,
    [TDEThumbprint]         VARBINARY(32),
    [SnapshotURL]           NVARCHAR(360)
)

INSERT INTO @fileListTable
EXEC('RESTORE FILELISTONLY FROM DISK = ''%s/%s''')
SET @sql = 'RESTORE DATABASE [%s] FROM DISK = ''%s/%s'' WITH '
SELECT @sql = @sql + char(13) + ' MOVE ''' + LogicalName + ''' TO ''/var/opt/mssql/' + LogicalName + '.' + RIGHT(PhysicalName,CHARINDEX('\',PhysicalName)) + ''','
FROM @fileListTable
WHERE IsPresent = 1
SET @sql = SUBSTRING(@sql, 1, LEN(@sql)-1)
EXEC(@sql)`

		query(fmt.Sprintf(text, temporaryFolder, file, dbNameAsIdentifier, temporaryFolder, file))
	} else if filepath.Ext(f) == ".mdf" {
		// Attach database
		output.Infof("Attaching database %s", databaseName)

		controller.RunCmdInContainer(containerId, []string{"chown", "mssql:root", temporaryFolder + "/" + file})
		controller.RunCmdInContainer(containerId, []string{"chmod", "-o-r", temporaryFolder + "/" + file})
		controller.RunCmdInContainer(containerId, []string{"chmod", "-u+rw", temporaryFolder + "/" + file})
		controller.RunCmdInContainer(containerId, []string{"chmod", "-g+r", temporaryFolder + "/" + file})

		text := `SET NOCOUNT ON;

CREATE DATABASE [%s]   
    ON (FILENAME = '%s/%s'), (FILENAME = '/var/opt/mssql/data/StackOverflow2010_log.ldf')
    FOR ATTACH;`

		query(fmt.Sprintf(text, dbNameAsIdentifier, temporaryFolder, file))
	} else if filepath.Ext(f) == ".bacpac" {
		controller.DownloadFile(
			containerId,
			"https://aka.ms/sqlpackage-linux",
			"/tmp",
		)

		controller.RunCmdInContainer(containerId, []string{"apt-get", "update"})
		controller.RunCmdInContainer(containerId, []string{"apt-get", "install", "-y", "unzip"})
		controller.RunCmdInContainer(containerId, []string{"unzip", "/tmp/sqlpackage-linux", "-d", "/opt/sqlpackage"})
		controller.RunCmdInContainer(containerId, []string{"rm", "/tmp/sqlpackage-linux"})
		controller.RunCmdInContainer(containerId, []string{"chmod", "+x", "/opt/sqlpackage/sqlpackage"})
		controller.RunCmdInContainer(containerId, []string{
			"/opt/sqlpackage/sqlpackage",
			"/Action:import",
			"/SourceFile:" + temporaryFolder + "/" + file,
			"/TargetUser:" + userName,
			"/TargetPassword:" + password,
			"/TargetServerName:localhost",
			"/TargetDatabaseName:" + dbNameAsIdentifier,
			"/TargetTrustServerCertificate:true"})
	}

	alterDefaultDb := fmt.Sprintf(
		"ALTER LOGIN [%s] WITH DEFAULT_DATABASE = [%s]",
		userName,
		dbNameAsNonIdentifier)
	query(alterDefaultDb)
}

func getDbNameAsIdentifier(dbName string) string {
	escapedDbNAme := strings.ReplaceAll(dbName, "'", "''")
	return strings.ReplaceAll(escapedDbNAme, "]", "]]")
}

func getDbNameAsNonIdentifier(dbName string) string {
	return strings.ReplaceAll(dbName, "]", "]]")
}

// parseDbName returns the databaseName from --using arg
// It sets database name to the specified database name
// or in absence of it, it is set to the filename without
// extension.
func parseDbName(usingDbUrl string) string {
	u, _ := url.Parse(usingDbUrl)
	dbToken := path.Base(u.Path)
	if dbToken != "." && dbToken != "/" {
		lastIdx := strings.LastIndex(dbToken, ".bak")
		if lastIdx == -1 {
			lastIdx = strings.LastIndex(dbToken, ".mdf")
		}
		if lastIdx != -1 {
			//Get file name without extension
			fileName := dbToken[0:lastIdx]
			lastIdx += 5
			if lastIdx >= len(dbToken) {
				return fileName
			}
			//Return database name if it was specified
			return dbToken[lastIdx:]
		} else {
			lastIdx := strings.LastIndex(dbToken, ".bacpac")
			if lastIdx != -1 {
				//Get file name without extension
				fileName := dbToken[0:lastIdx]
				lastIdx += 8
				if lastIdx >= len(dbToken) {
					return fileName
				}
				//Return database name if it was specified
				return dbToken[lastIdx:]
			} else {
				lastIdx := strings.LastIndex(dbToken, ".7z")
				if lastIdx != -1 {
					//Get file name without extension
					fileName := dbToken[0:lastIdx]
					lastIdx += 4
					if lastIdx >= len(dbToken) {
						return fileName
					}
					//Return database name if it was specified
					return dbToken[lastIdx:]
				}
			}
		}

	}
	return ""
}

func extractUrl(usingArg string) string {
	urlEndIdx := strings.LastIndex(usingArg, ".bak")
	if urlEndIdx == -1 {
		urlEndIdx = strings.LastIndex(usingArg, ".mdf")
	}
	if urlEndIdx != -1 {
		return usingArg[0:(urlEndIdx + 4)]
	}

	if urlEndIdx == -1 {
		urlEndIdx = strings.LastIndex(usingArg, ".7z")
		if urlEndIdx != -1 {
			return usingArg[0:(urlEndIdx + 3)]
		}
	}

	if urlEndIdx == -1 {
		urlEndIdx = strings.LastIndex(usingArg, ".bacpac")
		if urlEndIdx != -1 {
			return usingArg[0:(urlEndIdx + 7)]
		}
	}

	return usingArg
}