package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDbNameIfExists(t *testing.T) {

	type test struct {
		input          string
		expectedOutput string
	}

	tests := []test{
		// Positive Testcases
		// Database name specified
		{"https://example.com/my%20random%20bac%27kp%5Dack.bak,myDbName", "myDbName"},
		{"https://example.com/my%20random%20bac%27kp%5Dack.bak,myDb Name", "myDb Name"},
		{"https://example.com/my%20random%20bac%27kp%5Dack.bak,myDb Na,me", "myDb Na,me"},
		{"https://example.com/my%20random%20bac%27kp%5Dack.bak,[myDb Na,me]", "[myDb Na,me]]"},
		{"https://example.com/my%20random%20bac%27kp%5Dack.bak,[myDb Na'me]", "[myDb Na''me]]"},
		{"https://example.com/my%20random%20bac%27kp%5Dack.bak,[myDb ,Nam,e]", "[myDb ,Nam,e]]"},

		// Delimiter between filename and databaseName is part of the filename
		// Decoded filename: my random .bak bac'kp]ack.bak
		{"https://example.com/my%20random%20.bak%20bac%27kp%5Dack.bak,[myDb ,Nam,e]", "[myDb ,Nam,e]]"},

		// Database name not specified
		{"https://example.com/my%20random%20.bak%20bac%27kp%5Dack.bak", "my random .bak bac''kp]]ack"},
		{"https://example.com/my%20random%20.bak%20bac%27kp%5Dack.bak,", "my random .bak bac''kp]]ack"},

		//Negative Testcases
		{"https://example.com,myDbName", ""},
	}

	for _, testcase := range tests {
		dbname := parseDbName(testcase.input)
		dbname = getEscapedDbName(dbname)
		assert.Equal(t, testcase.expectedOutput, dbname, "Unexpected value from getDbNameForScripts")
	}
}
