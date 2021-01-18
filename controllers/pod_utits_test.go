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
   77 root      0:16 /usr/bin/java -jar /app/app.jar
  238 root      0:00 /bin/sh
  244 root      0:00 ps
`

	pids, err := extractMatchedPids(stdout, "java -jar")
	assert.NoError(err)

	assert.Equal(pids, []int{77})

	stdout = `
    PID TTY      STAT   TIME COMMAND
      1 ?        Ssl    0:10 java -jar /app/app.jar
    147 pts/0    Ss     0:00 /bin/bash
    161 pts/0    R+     0:00 ps -x
`

	pids, err = extractMatchedPids(stdout, "java -jar")
	assert.NoError(err)

	assert.Equal(pids, []int{1})
}
