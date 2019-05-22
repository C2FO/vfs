/*
	testsuite is meant to be run by implementors of backends to ensure that the behaviors of their backend matches the
	expected behavior of the interface.  Note you my need to pass additional environmental variables for authentication.

	VFS_INTEGRATION_LOCATIONS="file:///tmp/vfs_test/;s3://c2foupload-test/vfs_test/;gs://enterprise-test/vfs_test/" \
	GOOGLE_APPLICATION_CREDENTIALS=~/.gsutil/account.json \
	AWS_REGION=us-west-2 \
	go test ./backend/testsuite
*/
package testsuite