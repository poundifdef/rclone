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
	//fmt.Println("Listing...", f.root, dirName, p)

	dirEntries := []Object{}

	file1 := Object{
		ID:     "1",
		Name:   "File 1",
		fs:     f,
		Parent: "",
		Type:   "FILE",
	}
	dirEntries = append(dirEntries, file1)

	file2 := Object{
		ID:     "2",
		Name:   "File 2",
		fs:     f,
		Parent: "",
		Type:   "FILE",
	}
	dirEntries = append(dirEntries, file2)

	file3 := Object{
		ID:     "3",
		Name:   "Dir 3",
		fs:     f,
		Parent: "",
		Type:   "FOLDER",
	}
	dirEntries = append(dirEntries, file3)

	file4 := Object{
		ID:     "4",
		Name:   "Dir 4",
		fs:     f,
		Parent: "",
		Type:   "FOLDER",
	}
	dirEntries = append(dirEntries, file4)

	file5 := Object{
		ID:     "5",
		Name:   "File 5",
		fs:     f,
		Parent: "3",
		Type:   "FILE",
	}
	dirEntries = append(dirEntries, file5)
	/*

		dirEntry := fs.NewDir("Dir 3", time.Now()).SetID("3")
		dirEntries = append(dirEntries, dirEntry)

		dirEntry2 := fs.NewDir("Dir 4", time.Now()).SetID("4")
		dirEntries = append(dirEntries, dirEntry2)

		file5 := &Object{
			ID:     "5",
			Name:   "File 5",
			fs:     f,
			Parent: "Dir 3",
		}
		dirEntries = append(dirEntries, file5)

		dirMap := make(map[string][]string)
			for _, d := range dirEntries {
				dirMap[d.String()]
				//fmt.Println("full path", d.Remote())
			}

		p := path.Join(f.root, dirName)
	*/
	//rootDir := f.root
	/*
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
	*/

	//nodes := make(map[string]Object)

	/*
		type node struct {
			V        Object
			Children []Object
		}
	*/

	dirMap := make(map[string][]Object)
	for _, d := range dirEntries {
		dirMap[d.Parent] = append(dirMap[d.Parent], d)

		/*
			fmt.Println("Remote..." + d.Remote())
			if strings.HasPrefix(d.Remote(), p) {
				rc = append(rc, d)
			}
		*/
	}

	levels := []string{""}
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

	//fmt.Println(levels)
	//fmt.Println(dirMap)
	// a/b/c

	parentAsset := Object{}
	children := dirMap[""]
	for j, asset := range children {
		children[j].remote = asset.Name
	}
	for i, level := range levels {
		if i == 0 {
			continue
		}

		//fmt.Println(i, len(levels), len(children))
		if i < len(levels) && len(children) == 0 {
			return nil, fs.ErrorDirNotFound
		}

		for j, asset := range children {
			remote := strings.Join(levels[1:i+1], "/")
			remote = strings.TrimPrefix(remote, f.root)

			if remote == "" {
				remote = asset.Name
			} else {
				remote = remote + "/" + asset.Name
			}
			//fmt.Println(asset.Name, remote)

			children[j].remote = remote

			if asset.Name == level {
				parentAsset = asset
				children = dirMap[asset.ID]
				break
			} else {
				if j == len(children)-1 {
					return nil, fs.ErrorDirNotFound
				}
			}
		}
	}

	fmt.Println(parentAsset)
	for _, v := range dirMap {
		for _, a := range v {
			fmt.Println(a.Name, a.remote)
		}
	}
	//fmt.Println(parentAsset, children)
	rc := []fs.DirEntry{}
	if parentAsset.Type == "FILE" {
		rc = append(rc, &parentAsset)
	} else {
		for _, child := range children {
			if child.Type == "FILE" {
				c := child
				rc = append(rc, &c)
			}
			/*
				if child.Type == "FOLDER" {
					if parentAsset.Name == "" {
						dirEntry := fs.NewDir(child.Name, time.Now()).SetID(child.ID)
						rc = append(rc, dirEntry)
					} //else {
					//dirEntry := fs.NewDir(parentAsset.Name+"/"+child.Name, time.Now()).SetID(child.ID)
					//rc = append(rc, dirEntry)

					//}
				}
			*/
		}
	}
	/*
		if dirName == "" {
			newDirName := path.Join(dirName, "d1")
			dirEntry := fs.NewDir(newDirName, time.Now()).SetID("2")
			dirEntries = append(dirEntries, dirEntry)
		}
	*/

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
	return o.remote
	/*
		if o.Parent == "" {
			return o.Name
		}

		return path.Join(o.ParentName, o.Name)
	*/
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
		root:  root,
	}

	// TODO: look at fs.go:505 (Features struct)
	f.features = (&fs.Features{
		CanHaveEmptyDirectories: true,
		DuplicateFiles:          true,
	}).Fill(ctx, f)

	return f, nil
}
