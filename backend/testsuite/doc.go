/*
Package testsuite is meant to be run by implementors of backends to ensure that the behaviors of their backend matches the
expected behavior of the interface.  Note you may need to pass additional environmental variables for authentication.
You may include in a ; separated list any number of uri's whose scheme implementations will be tested.  Each URI
will be tested against every other URI for io.* and Move/Copy functions.

	VFS_INTEGRATION_LOCATIONS="file:///tmp/vfs_test/;mem://A/path/to/"
	go test -tags=vfsintegration ./backend/testsuite

NOTE: for safety, os-based scheme will not clean up after top level location in case some yahoo specified file:/// as the
test location.  All sub locations and files will be cleaned up(removed).
*/
package testsuite
