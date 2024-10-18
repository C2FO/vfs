# vfscp

---

vfscp copies a file from one place to another, even between supported remote
systems. Complete URI (scheme:// authority/path) required except for local
file system. See github.com/c2fo/vfs docs for authentication.

### Usage

vfscp's usage is extremely simple:

    vfscp <uri> <uri>
    -help   prints help message

### Examples

Local OS URI's can be expressed without a scheme:
```bash
    vfscp /some/local/file.txt s3://mybucket/path/to/myfile.txt
```
But may also be use the full scheme uri:
```bash
    vfscp file:///some/local/file.txt s3://mybucket/path/to/myfile.txt
```
Copy a file from Google Cloud Storage to Amazon S3
```bash
    vfscp gs://googlebucket/some/path/photo.jpg s3://awsS3bucket/path/to/photo.jpg
```
