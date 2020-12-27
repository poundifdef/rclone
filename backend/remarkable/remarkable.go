package remarkable

import (
	"bytes"
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

	rm2pdf "github.com/poundifdef/go-remarkable2pdf"
)

// Register with Fs
func init() {
	fs.Register(&fs.RegInfo{
		Name:        "remarkable",
		Description: "Remarkable Tablet",
		NewFs:       NewFs,

		// TODO: auto config this by taking one-time token

		// Token returned when you first call POST /device/new
		Options: []fs.Option{
			{
				Name:     "refresh",
				Help:     "Refresh token",
				Required: true,
			},
			{
				Name:    "convert_to_pdf",
				Help:    "Convert files to PDF. Default behavior is to sync the raw files from the API.",
				Default: "false",
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
	fs       *Fs // what this object is part of
	MD5      string
	FileSize int64
	remote   string
}

// Fs represents a remote b2 server
type Fs struct {
	name          string // name of this remote
	features      *fs.Features
	refresh       string
	convertToPDF  string
	srv           *rest.Client
	root          string
	fileListCache []*Object
	dirLineage    map[string][]string
	flatFiles     map[string]*Object
	dirMap        map[string]*Object
}

func (f *Fs) Name() string {
	return f.name
}

// Root of the remote (as passed into NewFs)
func (f *Fs) Root() string {
	return f.root
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
	if f.convertToPDF == "true" {
		return hash.Set(hash.None)
	}

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

	if o.fs.convertToPDF == "false" {
		return resp.Body, err
	}

	inputBytes, _ := ioutil.ReadAll(resp.Body)
	var buf bytes.Buffer

	err = rm2pdf.RenderRmNotebookFromBytes(inputBytes, &buf)
	if err != nil {
		return nil, err
	}
	return ioutil.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

func (o *Object) Remove(ctx context.Context) error {
	return nil
}

func (f *Fs) generateFileTree() {
	for i, d := range f.fileListCache {
		f.dirMap[d.ID] = f.fileListCache[i]
	}

	for _, d := range f.fileListCache {
		fullPath := []string{}
		parent := d.Parent

		for {
			if parent == "" {
				break
			}

			fullPath = append([]string{parent}, fullPath...)
			parentObj, ok := f.dirMap[parent]
			if !ok {
				break
			}
			parent = parentObj.Parent

		}
		fullPath = append(fullPath, d.ID)
		f.dirLineage[d.ID] = fullPath
	}

	for _, lineage := range f.dirLineage {
		path := ""
		for i, node := range lineage {
			path += f.dirMap[node].VissibleName
			if i < len(lineage)-1 {
				path += "/"
			}
		}
		f.flatFiles[path] = f.dirMap[lineage[len(lineage)-1]]

		a := f.dirMap[lineage[len(lineage)-1]]
		a.remote = path
	}
}

func (f *Fs) refreshFiles(ctx context.Context) error {
	// Fetch file list from Temarkable API

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
	resp, err := f.srv.CallJSON(ctx, &opts, nil, &rl)
	if err != nil {
		return err
	}
	f.fileListCache = rl
	resp.Body.Close()

	for _, item := range f.fileListCache {
		item.fs = f
	}

	trashObject := &Object{
		ID:           "trash",
		Parent:       "",
		VissibleName: "Trash",
		fs:           f,
		Type:         "CollectionType",
	}
	f.fileListCache = append(f.fileListCache, trashObject)

	f.generateFileTree()

	return nil
}
func (f *Fs) List(ctx context.Context, dirName string) (entries fs.DirEntries, err error) {
	// TODO: check errors
	if f.fileListCache == nil {
		err = f.refreshFiles(ctx)
		if err != nil {
			return
		}
	}

	levels := []string{}
	if f.root != "" {
		if f.root[0] == '/' {
			levels = append(levels, strings.Split(f.root[1:], "/")...)
		} else {
			levels = append(levels, strings.Split(f.root, "/")...)
		}
		if levels[len(levels)-1] == "" {
			levels = levels[:len(levels)-1]
		}
	}
	if dirName != "" {
		if dirName[0] == '/' {
			levels = append(levels, strings.Split(dirName[1:], "/")...)
		} else {
			levels = append(levels, strings.Split(dirName, "/")...)
		}
		if levels[len(levels)-1] == "" {
			levels = levels[:len(levels)-1]
		}
	}

	fullPath := strings.Join(levels, "/")

	if fullPath == "" {
		for k, v := range f.dirLineage {
			if len(v) == 1 {
				f := f.dirMap[k]

				if f.Type == "DocumentType" {
					entries = append(entries, f)
				}

				if f.Type == "CollectionType" {
					dirEntry := fs.NewDir(f.remote, time.Now()).SetID(f.ID)
					entries = append(entries, dirEntry)
				}
			}
		}

	} else {
		thisPath, ok := f.flatFiles[fullPath]
		if !ok {
			return nil, fs.ErrorDirNotFound
		}

		if thisPath.Type == "DocumentType" {
			entries = append(entries, thisPath)
		}

		if thisPath.Type == "CollectionType" {
			for k1, v1 := range f.flatFiles {
				if k1 == fullPath {
					continue
				}
				if v1.remote == fullPath+"/"+v1.VissibleName {
					if v1.Type == "DocumentType" {
						entries = append(entries, v1)
					}
					if v1.Type == "CollectionType" {
						dirEntry := fs.NewDir(v1.Remote(), time.Now()).SetID(v1.ID)
						entries = append(entries, dirEntry)
					}
				}
			}
		}
	}

	return
}

func (o *Object) Fs() fs.Info {
	return o.fs
}

func (o *Object) String() string {
	return o.VissibleName
}

// Remote returns the remote path
func (o *Object) Remote() string {
	if o.fs.root == o.remote {
		return o.VissibleName
	}

	return strings.TrimPrefix(o.remote, o.fs.root+"/")
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
	if o.fs.convertToPDF == "true" {
		return -1
	}

	if o.FileSize == 0 {
		o.getDetails()
	}
	return o.FileSize
}

// Hash returns the selected checksum of the file
// If no checksum is available it returns ""
func (o *Object) Hash(ctx context.Context, ty hash.Type) (string, error) {
	if o.fs.convertToPDF == "true" {
		return "", hash.ErrUnsupported
	}

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
	convertToPDF, _ := m.Get("convert_to_pdf")

	f := &Fs{
		name:         name,
		refresh:      refresh,
		convertToPDF: convertToPDF,
		srv:          rest.NewClient(fshttp.NewClient(ctx)),
		root:         strings.Trim(root, "/"),
		dirLineage:   make(map[string][]string),
		flatFiles:    make(map[string]*Object),
		dirMap:       make(map[string]*Object),
	}

	// TODO: look at fs.go:505 (Features struct)
	f.features = (&fs.Features{
		CanHaveEmptyDirectories: true,
		DuplicateFiles:          true,
	}).Fill(ctx, f)

	return f, nil
}
