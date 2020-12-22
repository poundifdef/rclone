package remarkable

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/fshttp"
	"github.com/rclone/rclone/fs/hash"
	"github.com/rclone/rclone/lib/rest"
)

// Register with Fs
func init() {
	fs.Register(&fs.RegInfo{
		Name:        "remarkable",
		Description: "Remarkable Tablet",
		NewFs:       NewFs,

		// TODO: auto config this by taking one-time token

		// Token returned when you first call POST /device/new
		Options: []fs.Option{{
			Name:     "refresh",
			Help:     "Refresh token",
			Required: true,
		},
		},
	})
}

type Object struct {
	// Comes back from remarkable API
	ID                string
	Version           int
	Message           string
	Success           bool
	BlobURLGet        string
	BlobURLGetExpires string
	ModifiedClient    string
	Type              string
	VissibleName      string
	CurrentPage       int
	Bookmarked        bool
	Parent            string

	// internally used
	fs         *Fs // what this object is part of
	ParentName string
	MD5        string
	FileSize   int64
}

// Fs represents a remote b2 server
type Fs struct {
	name          string // name of this remote
	features      *fs.Features
	refresh       string
	srv           *rest.Client
	root          string
	fileListCache []*Object
}

func (f *Fs) Name() string {
	return f.name
}

// Root of the remote (as passed into NewFs)
func (f *Fs) Root() string {
	return ""
}

// String returns a description of the FS
func (f *Fs) String() string {
	return f.name
}

// Precision of the ModTimes in this Fs
func (f *Fs) Precision() time.Duration {
	return time.Second
}

// Returns the supported hash types of the filesystem
func (f *Fs) Hashes() hash.Set {
	return hash.Set(hash.MD5)
}

// Features returns the optional features of this Fs
func (f *Fs) Features() *fs.Features {
	return f.features
}

// NewObject finds the Object at remote.  If it can't be found
// it returns the error ErrorObjectNotFound.
func (f *Fs) NewObject(ctx context.Context, remote string) (fs.Object, error) {
	return &Object{}, nil
}

// Put in to the remote path with the modTime given of the given size
//
// When called from outside an Fs by rclone, src.Size() will always be >= 0.
// But for unknown-sized objects (indicated by src.Size() == -1), Put should either
// return an error or upload it properly (rather than e.g. calling panic).
//
// May create the object even if it returns an error - if so
// will return the object and the error, otherwise will return
// nil and the error
func (f *Fs) Put(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	return &Object{}, nil

}

func (o *Object) Update(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) error {
	return nil
}

func (o *Object) SetModTime(ctx context.Context, t time.Time) error {
	return nil
}

func (o *Object) Open(ctx context.Context, options ...fs.OpenOption) (io.ReadCloser, error) {
	opts := rest.Opts{
		Method:  "GET",
		RootURL: o.BlobURLGet,
	}
	resp, err := o.fs.srv.Call(context.TODO(), &opts)
	return resp.Body, err
}

func (o *Object) Remove(ctx context.Context) error {
	return nil
}

func (f *Fs) List(ctx context.Context, dirName string) (entries fs.DirEntries, err error) {

	// TODO: check errors
	if f.fileListCache == nil {
		log.Println("fetching file list...")
		opts := rest.Opts{
			Method:  "POST",
			Path:    "/token/json/2/user/new",
			RootURL: "https://my.remarkable.com",
			ExtraHeaders: map[string]string{
				"Authorization": "Bearer " + f.refresh,
			},
		}
		resp, _ := f.srv.Call(ctx, &opts)
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		authToken := string(bodyBytes)
		resp.Body.Close()

		// TODO: use service discovery to check the root URL
		v := url.Values{}
		v.Set("withBlob", "true")
		opts = rest.Opts{
			Method:  "GET",
			Path:    "/document-storage/json/2/docs",
			RootURL: "https://document-storage-production-dot-remarkable-production.appspot.com",
			ExtraHeaders: map[string]string{
				"Authorization": "Bearer " + authToken,
			},
			Parameters: v,
		}

		rl := []*Object{}
		resp, err = f.srv.CallJSON(ctx, &opts, nil, &rl)
		f.fileListCache = rl
		resp.Body.Close()

	}

	dirEntries := []fs.DirEntry{}

	if dirName == "" {
		for i, rItem := range f.fileListCache {
			if rItem.Parent == dirName {
				if rItem.Type == "DocumentType" {
					f.fileListCache[i].fs = f
					f.fileListCache[i].ParentName = f.root
					dirEntries = append(dirEntries, rItem)

				}
			}
		}
	}
	/*
		if dirName == "" {
			newDirName := path.Join(dirName, "d1")
			dirEntry := fs.NewDir(newDirName, time.Now()).SetID("2")
			dirEntries = append(dirEntries, dirEntry)
		}
	*/
	return dirEntries, nil
}

func (o *Object) Fs() fs.Info {
	return o.fs
}

func (o *Object) String() string {
	return o.VissibleName
}

// Remote returns the remote path
func (o *Object) Remote() string {
	if o.Parent == "" {
		return o.VissibleName
	}
	return o.ParentName + "/" + o.VissibleName
}

func (o *Object) getDetails() {
	opts := rest.Opts{
		Method:  "HEAD",
		RootURL: o.BlobURLGet,
	}

	resp, err := o.fs.srv.Call(context.TODO(), &opts)
	resp.Body.Close()
	if err != nil {
		log.Println(err)
		return
	}
	hashes := resp.Header.Values("x-goog-hash")
	for _, hash := range hashes {
		if strings.HasPrefix(hash, "md5") {
			data, _ := base64.StdEncoding.DecodeString(hash)
			o.MD5 = fmt.Sprintf("%x", data)
		}
	}
	cl := resp.Header.Get("content-length")
	o.FileSize, _ = strconv.ParseInt(cl, 10, 64)
}

// ModTime returns the modification date of the file
// It should return a best guess if one isn't available
func (o *Object) ModTime(ctx context.Context) time.Time {
	layout := "2006-01-02T15:04:05"
	t, _ := time.Parse(layout, o.ModifiedClient[:19])
	return t
}

// Size returns the size of the file
func (o *Object) Size() int64 {
	if o.FileSize == 0 {
		o.getDetails()
	}
	return o.FileSize
}

// Hash returns the selected checksum of the file
// If no checksum is available it returns ""
func (o *Object) Hash(ctx context.Context, ty hash.Type) (string, error) {
	if o.FileSize == 0 {
		o.getDetails()
	}

	if ty == hash.MD5 {
		return o.MD5, nil
	}

	return "", hash.ErrUnsupported
}

// Storable says whether this object can be stored
func (o *Object) Storable() bool {
	return true
}

// Mkdir makes the directory (container, bucket)
//
// Shouldn't return an error if it already exists
func (f *Fs) Mkdir(ctx context.Context, dir string) error {
	return nil

}

// Rmdir removes the directory (container, bucket) if empty
//
// Return an error if it doesn't exist or isn't empty
func (f *Fs) Rmdir(ctx context.Context, dir string) error {
	return nil

}

func NewFs(ctx context.Context, name, root string, m configmap.Mapper) (fs.Fs, error) {
	// TODO: check for errors, return an error if we can't instantiate
	refresh, _ := m.Get("refresh")

	f := &Fs{
		name:    name,
		refresh: refresh,
		srv:     rest.NewClient(fshttp.NewClient(ctx)),
		root:    root,
	}

	// TODO: look at fs.go:505 (Features struct)
	f.features = (&fs.Features{
		CanHaveEmptyDirectories: true,
		DuplicateFiles:          true,
	}).Fill(ctx, f)

	return f, nil
}
