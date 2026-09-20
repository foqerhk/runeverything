//go:build windows && embedagent

package main

import _ "embed"

//go:embed embedded/runeverything.exe
var agentBinary []byte

func embeddedAgent() ([]byte, error) {
	if len(agentBinary) == 0 {
		return nil, errEmptyEmbed
	}
	return agentBinary, nil
}

type embedErr string

func (e embedErr) Error() string { return string(e) }

const errEmptyEmbed = embedErr("embedded agent binary is empty")
