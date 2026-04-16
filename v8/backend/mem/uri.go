package mem

import "fmt"

func formatFileURI(f *File) string {
	return fmt.Sprintf("%s://%s%s", f.Location().FileSystem().Scheme(), f.Location().Authority().String(), f.Path())
}

func formatLocationURI(l *Location) string {
	return fmt.Sprintf("%s://%s%s", l.FileSystem().Scheme(), l.Authority().String(), l.Path())
}
