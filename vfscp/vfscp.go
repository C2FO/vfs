package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/c2fo/vfs/v6/utils"
	"github.com/c2fo/vfs/v6/vfssimple"
)

const usageTemplate = `
%[1]s copies a file from one place to another, even between supported remote systems.
Complete URI (scheme://authority/path) required except for local filesystem.
See github.com/c2fo/vfs docs for authentication.

Usage:  %[1]s <uri> <uri>

    ie,        %[1]s /some/local/file.txt s3://mybucket/path/to/myfile.txt
    same as    %[1]s file:///some/local/file.txt s3://mybucket/path/to/myfile.txt
    gcs to s3  %[1]s gs://googlebucket/some/path/photo.jpg s3://awsS3bucket/path/to/photo.jpg

    -help
        prints this message

`

func main() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stdout, usageTemplate, os.Args[0])
	}
	var help bool
	flag.BoolVar(&help, "help", false, "prints this message")
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(0)
	}

	if len(flag.Args()) != 2 {
		flag.Usage()
		os.Exit(1)
	}

	fmt.Println("")

	srcFileURI, err := utils.PathToURI(flag.Arg(0))
	if err != nil {
		panic(err)
	}
	targetFileURI, err := utils.PathToURI(flag.Arg(1))
	if err != nil {
		panic(err)
	}

	copyFiles(srcFileURI, targetFileURI)
}

func copyFiles(srcFileURI, targetFileURI string) {
	green := color.New(color.FgHiGreen).Add(color.Bold)

	copyMessage(srcFileURI, targetFileURI)

	srcFile, err := vfssimple.NewFile(srcFileURI)
	if err != nil {
		failMessage(err)
	}
	targetFile, err := vfssimple.NewFile(targetFileURI)
	if err != nil {
		failMessage(err)
	}
	err = srcFile.CopyToFile(targetFile)
	if err != nil {
		failMessage(err)
	}

	fmt.Print(green.Sprint("done\n\n"))
}

func failMessage(err error) {
	red := color.New(color.FgHiRed).Add(color.Bold)
	fmt.Printf(red.Sprint("failed\n\n")+"\n%s\n\n", err.Error())
	os.Exit(1)
}

func copyMessage(src, dest string) {
	white := color.New(color.FgHiWhite).Add(color.Bold)
	blue := color.New(color.FgHiBlue).Add(color.Bold)
	fmt.Print(white.Sprint("Copying ") +
		blue.Sprint(src) +
		white.Sprint(" to ") +
		blue.Sprint(dest) +
		white.Sprint(" ... "))
}
