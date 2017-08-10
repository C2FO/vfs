package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/c2fo/vfs/vfssimple"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "vfscp"
	app.Usage = "Copies a file from one place to another, even between supported remote systems"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "awsKeyId",
			Usage:  "aws access key id for user",
			EnvVar: "AWS_ACCESS_KEY_ID",
		},
		cli.StringFlag{
			Name:   "awsSecretKey",
			Usage:  "aws secret key for user",
			EnvVar: "AWS_ACCESS_KEY",
		},
		cli.StringFlag{
			Name:   "awsSessionToken",
			Usage:  "aws session token",
			EnvVar: "AWS_SESSION_TOKEN",
		},
		cli.StringFlag{
			Name:   "awsRegion",
			Usage:  "aws region",
			EnvVar: "AWS_REGION",
		},
	}
	app.Action = func(c *cli.Context) error {
		err := checkArgs(c.Args().Get(0), c.Args().Get(1))
		if err != nil {
			return err
		}
		srcFileURI, targetFileURI, err := normalizeArgs(c)
		// TODO: if file is empty, create an empty file at targetFile. This should probably be done by vfs by default
		// TODO: add support for S3 URIs. All relative paths or otherwise incomplete URIs should be interpreted as local paths.
		fmt.Println(fmt.Sprintf("Copying %s to %s", srcFileURI, targetFileURI))
		srcFile, _ := vfssimple.NewFile(srcFileURI)
		targetFile, _ := vfssimple.NewFile(targetFileURI)
		return srcFile.CopyToFile(targetFile)
	}

	app.Run(os.Args)
}

func checkArgs(a1, a2 string) error {
	if a1 == "" || a2 == "" {
		return errors.New("vfscp requires 2 non-empty arguments")
	}
	return nil
}

func normalizeArgs(c *cli.Context) (string, string, error) {
	a1 := c.Args().Get(0)
	a2 := c.Args().Get(1)
	normalizedArgs := make([]string, 2)
	for i, a := range []string{a1, a2} {
		u, err := url.Parse(a)
		if err != nil {
			return "", "", err
		}
		if u.IsAbs() {
			normalizedArgs[i] = a
			if err := initializeFS(u.Scheme, c); err != nil {
				return "", "", err
			}
		} else {
			absPath, err := filepath.Abs(a)
			if err != nil {
				return "", "", err
			}
			normalizedArgs[i] = "file://" + absPath
			if err := initializeFS("file", c); err != nil {
				return "", "", err
			}
		}
	}
	return normalizedArgs[0], normalizedArgs[1], nil
}

func initializeFS(scheme string, c *cli.Context) error {
	switch scheme {
	case "gs":
		return vfssimple.InitializeGSFileSystem()
	case "s3":
		return vfssimple.InitializeS3FileSystem(c.String("awsKeyId"), c.String("awsSecretKey"), c.String("awsRegion"), c.String("awsSessionToken"))
	case "file":
		vfssimple.InitializeLocalFileSystem()
	}
	return nil
}
