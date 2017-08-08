// This file contains unified error handling for vfs as well as a technique for handling
// multiple errors in the event of deferred method calls such as file.Close()
package vfs

import (
	"errors"
	"fmt"
)

// MultiErr provides a set of functions to handle the scenario where, because of errors in defers,
// we have a way to handle the potenetial of multiple errors. For instance, if you do a open a file,
// defer it's close, then fail to Seek. The seek fauilure has one error but then the Close fails as
// well. This ensure neither are ignored.
type MultiErr struct {
	errs []error
}

// Constructor for generating a zero-value MultiErr reference.
func NewMutliErr() *MultiErr {
	return &MultiErr{}
}

// Returns the error message string.
func (me *MultiErr) Error() string {
	var errorString string
	for _, err := range me.errs {
		errorString = fmt.Sprintf("%s%s\n", errorString, err.Error())
	}
	return errorString
}

// Appends the provided errors to the errs slice for future message reporting.
func (me *MultiErr) Append(errs ...error) error {
	me.errs = append(me.errs, errs...)
	return errors.New("return value for multiErr must be set in the first deferred function")
}

// If there are no errors in the MultErr instance, then return nil, otherwise return the full MultiErr instance.
func (me *MultiErr) OrNil() error {
	if len(me.errs) > 0 {
		return me
	}
	return nil
}

type singleErrReturn func() error

func (me *MultiErr) DeferFunc(f singleErrReturn) {
	if err := f(); err != nil {
		_ = me.Append(err)
	}
}
