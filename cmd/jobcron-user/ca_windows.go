package main

import "errors"

// Unix custody is the production operator contract; fail closed on Windows.
func checkCAFile(string) error {
	return errors.New("user: verified operator CA custody requires Unix")
}
