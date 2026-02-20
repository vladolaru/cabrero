package main

import _ "embed"

//go:embed hooks/pre-compact-backup.sh
var preCompactHookScript string

//go:embed hooks/session-end.sh
var sessionEndHookScript string
