package skeleton

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
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
		Name:        "skeleton",
		Description: "Skeleton Backend",
		NewFs:       NewFs,

		// TODO: auto config this by taking one-time token

		// Token returned when you first call POST /device/new
		Options: []fs.Option{{
			Name:     "hello_text",
			Help:     "Text for hello world",
			Required: true,
		},
		},
	})
}

type Object struct {
	ID   string
	Name string
	Type string

	// internally used
	fs     *Fs // what this object is part of
	Parent string
	remote string
	//Children map[string]Object
}

// Fs represents a remote b2 server
type Fs struct {
	name     string // name of this remote
	features *fs.Features
	hello    string
	srv      *rest.Client
	root     string
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
	r := ioutil.NopCloser(bytes.NewReader([]byte(o.fs.hello)))
	return r, nil
}

func (o *Object) Remove(ctx context.Context) error {
	return nil
}

func (f *Fs) List(ctx context.Context, dirName string) (entries fs.DirEntries, err error) {
	dirName = strings.Trim(dirName, "/")

	dirEntries := []*Object{}

	file1 := Object{
		ID:     "1",
		Name:   "File 1",
		fs:     f,
		Parent: "",
		Type:   "FILE",
	}
	dirEntries = append(dirEntries, &file1)

	file2 := Object{
		ID:     "2",
		Name:   "File 2",
		fs:     f,
		Parent: "",
		Type:   "FILE",
	}
	dirEntries = append(dirEntries, &file2)

	file3 := Object{
		ID:     "3",
		Name:   "Dir 3",
		fs:     f,
		Parent: "",
		Type:   "FOLDER",
	}
	dirEntries = append(dirEntries, &file3)

	file4 := Object{
		ID:     "4",
		Name:   "Dir 4",
		fs:     f,
		Parent: "",
		Type:   "FOLDER",
	}
	dirEntries = append(dirEntries, &file4)

	file5 := Object{
		ID:     "5",
		Name:   "File 5",
		fs:     f,
		Parent: "3",
		Type:   "FILE",
	}
	dirEntries = append(dirEntries, &file5)

	file6 := Object{
		ID:     "6",
		Name:   "Dir 6",
		fs:     f,
		Parent: "3",
		Type:   "FOLDER",
	}
	dirEntries = append(dirEntries, &file6)

	file7 := Object{
		ID:     "7",
		Name:   "File 7",
		fs:     f,
		Parent: "6",
		Type:   "FILE",
	}
	dirEntries = append(dirEntries, &file7)

	dirMap := make(map[string]*Object)
	for i, d := range dirEntries {
		dirMap[d.ID] = dirEntries[i]
	}

	dirLineage := make(map[string][]string)
	for _, d := range dirEntries {
		fullPath := []string{}
		parent := d.Parent

		for {
			if parent == "" {
				break
			}

			fullPath = append([]string{parent}, fullPath...)
			parentObj, ok := dirMap[parent]
			if !ok {
				break
			}
			parent = parentObj.Parent

		}
		fullPath = append(fullPath, d.ID)
		dirLineage[d.ID] = fullPath
	}

	flatFiles := make(map[string]*Object)
	for _, lineage := range dirLineage {
		path := ""
		for i, node := range lineage {
			path += dirMap[node].Name
			if i < len(lineage)-1 {
				path += "/"
			}
		}
		flatFiles[path] = dirMap[lineage[len(lineage)-1]]

		a := dirMap[lineage[len(lineage)-1]]
		a.remote = path
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

	//fmt.Println(f.root, "|", dirName)
	//fmt.Println(file1.remote)
	rc := []fs.DirEntry{}
	//return rc, nil
	fullPath := strings.Join(levels, "/")
	//fmt.Println(levels)
	//fmt.Println(dirLineage)

	if fullPath == "" {
		for k, v := range dirLineage {
			if len(v) == 1 {
				f := dirMap[k]

				if f.Type == "FILE" {
					rc = append(rc, f)
				}

				if f.Type == "FOLDER" {
					dirEntry := fs.NewDir(f.remote, time.Now()).SetID(f.ID)
					//dirEntry := fs.NewDir(f.Name, time.Now()).SetID(f.ID)
					rc = append(rc, dirEntry)
				}
			}
		}

	} else {
		//fmt.Println(dirName, fullPath)
		//fmt.Println(flatFiles)
		//if fullPath[0] == '/' {
		//fullPath = fullPath[1:]
		//}
		thisPath, ok := flatFiles[fullPath]
		if !ok {
			return nil, fs.ErrorDirNotFound
		}

		if thisPath.Type == "FILE" {
			rc = append(rc, thisPath)
		}

		if thisPath.Type == "FOLDER" {
			for k1, v1 := range flatFiles {
				if k1 == fullPath {
					continue
				}
				if v1.remote == fullPath+"/"+v1.Name {
					if v1.Type == "FILE" {
						rc = append(rc, v1)
					}
					if v1.Type == "FOLDER" {
						dirEntry := fs.NewDir(v1.Remote(), time.Now()).SetID(v1.ID)
						rc = append(rc, dirEntry)
					}
				}
			}
		}
	}

	return rc, nil
}

func (o *Object) Fs() fs.Info {
	return o.fs
}

func (o *Object) String() string {
	return o.Name
}

// Remote returns the remote path
func (o *Object) Remote() string {
	if o.fs.root == o.remote {
		return o.Name
	}

	return strings.TrimPrefix(o.remote, o.fs.root+"/")
}

// ModTime returns the modification date of the file
// It should return a best guess if one isn't available
func (o *Object) ModTime(ctx context.Context) time.Time {
	return time.Now()
}

// Size returns the size of the file
func (o *Object) Size() int64 {
	return -1
	//return int64(len(o.fs.hello))
}

// Hash returns the selected checksum of the file
// If no checksum is available it returns ""
func (o *Object) Hash(ctx context.Context, ty hash.Type) (string, error) {
	data := []byte(o.fs.hello)
	return fmt.Sprintf("%x", md5.Sum(data)), nil
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
	hello, _ := m.Get("hello_text")

	f := &Fs{
		name:  name,
		hello: hello,
		srv:   rest.NewClient(fshttp.NewClient(ctx)),
		root:  strings.Trim(root, "/"),
	}

	// TODO: look at fs.go:505 (Features struct)
	f.features = (&fs.Features{
		CanHaveEmptyDirectories: true,
		DuplicateFiles:          true,
	}).Fill(ctx, f)

	return f, nil
}
