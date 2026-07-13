package peercredentials

import "errors"

var ErrInsecurePrivateKey = errors.New("private key security policy is not satisfied")
