package onedrive

import "github.com/c2fo/vfs/contrib/backend/onedrive/types"

// ErrNotFound is returned (wrapped) by Client methods when the requested item does not exist.
var ErrNotFound = types.ErrNotFound

// Client defines the Microsoft Graph drive operations used by this backend. See types.Client.
type Client = types.Client

// UploadSession is an active resumable upload. See types.UploadSession.
type UploadSession = types.UploadSession

// DriveItem is the subset of Microsoft Graph driveItem metadata used by this backend.
type DriveItem = types.DriveItem
