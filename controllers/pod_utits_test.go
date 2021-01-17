package controllers

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExtractPids(t *testing.T) {
	assert := require.New(t)
	stdout := `
PID   USER     TIME  COMMAND
    1 root      0:00 /bin/ash /app/entrypoint.sh
    7 root      0:16 /usr/bin/java -jar /app/app.jar
  238 root      0:00 /bin/sh
  244 root      0:00 ps
`

	pids, err := extractMatchedPids(stdout, "java -jar")
	assert.NoError(err)

	assert.Equal(pids, []int{7})
}
