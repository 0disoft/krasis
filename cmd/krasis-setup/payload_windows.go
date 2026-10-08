//go:build windows && appsetup

package main

import _ "embed"

//go:embed payload/bundle.zip
var archive []byte

func payload() ([]byte, error) { return archive, nil }
