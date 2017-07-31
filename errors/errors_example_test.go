package errors

import "github.com/c2fo/vfs"

func ExampleMultiErr_DeferFunc() {
	// NOTE: We use a named error in the function since our first defer will set it based on any appended errors
	_ = func(f vfs.File) (rerr error) {
		//THESE LINES REQUIRED
		errs := NewMutliErr()
		defer func() { rerr = errs.OrNil() }()

		_, err := f.Read(nil)
		if err != nil {
			//for REGULAR ERROR RETURNS we just return the Appended errors
			return errs.Append(err)
		}

		// for defers, use DeferFunc and pass it the func name
		defer errs.DeferFunc(f.Close)

		_, err = f.Seek(0, 0)
		if err != nil {
			//for REGULAR ERROR RETURNS we just return the Appended errors
			return errs.Append(err)
		}

		return nil
	}
}
