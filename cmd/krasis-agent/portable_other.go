//go:build !windows

package main

func runPortableChild(args []string) (bool, int) {
	return len(args) > 0 && (args[0] == "product-portable-input-internal" || args[0] == "product-portable-guardian-internal"), 2
}
