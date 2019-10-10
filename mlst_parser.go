package goftp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type mlstParser struct{}

type mlstToken int

type mlstFacts struct {
	typ      string
	unixMode string
	perm     string
	size     string
	sizd     string
	modify   string
}

const (
	mlstFactName mlstToken = iota
	mlstFactValue
	mlstFilename
)

func parseMLST(entry string, skipSelfParent bool) (os.FileInfo, error) {
	return mlstParser{}.parse(entry, skipSelfParent)
}

// an entry looks something like this:
// type=file;size=12;modify=20150216084148;UNIX.mode=0644;unique=1000004g1187ec7; lorem.txt
func (p mlstParser) parse(entry string, skipSelfParent bool) (os.FileInfo, error) {
	var facts mlstFacts
	state := mlstFactName
	var left string // Previous token.
	var i1 int      // Current token's start position.
	for i2, r := range entry {
		switch r {
		case ';':
			if state == mlstFactValue {
				if left == "" {
					return nil, p.error(entry)
				}
				var (
					key = strings.ToLower(left[:len(left)-1])
					val = strings.ToLower(entry[i1:i2])
				)
				switch key {
				case "type":
					facts.typ = val
				case "unix.mode":
					facts.unixMode = val
				case "perm":
					facts.perm = val
				case "size":
					facts.size = val
				case "sizd":
					facts.sizd = val
				case "modify":
					facts.modify = val
				}
				if len(entry) >= i2+1 && entry[i2+1] == ' ' {
					state = mlstFilename
				} else {
					state = mlstFactName
				}
				i1 = i2 + 1
			}
		case '=':
			switch state {
			case mlstFactName:
				left = entry[i1 : i2+1]
				i1 = i2 + 1
				state = mlstFactValue
			}
		}
	}
	if state != mlstFilename || i1+1 >= len(entry) {
		return nil, p.error(entry)
	}
	filename := entry[i1+1:]

	typ := facts.typ

	if typ == "" {
		return nil, p.incompleteError(entry)
	}

	if skipSelfParent && (typ == "cdir" || typ == "pdir" || typ == "." || typ == "..") {
		return nil, nil
	}

	var mode os.FileMode
	if facts.unixMode != "" {
		m, err := strconv.ParseInt(facts.unixMode, 8, 32)
		if err != nil {
			return nil, p.error(entry)
		}
		mode = os.FileMode(m)
	} else if facts.perm != "" {
		// see http://tools.ietf.org/html/rfc3659#section-7.5.5
		for _, c := range facts.perm {
			switch c {
			case 'a', 'd', 'c', 'f', 'm', 'p', 'w':
				// these suggest you have write permissions
				mode |= 0200
			case 'l':
				// can list dir entries means readable and executable
				mode |= 0500
			case 'r':
				// readable file
				mode |= 0400
			}
		}
	} else {
		// no mode info, just say it's readable to us
		mode = 0400
	}

	if typ == "dir" || typ == "cdir" || typ == "pdir" {
		mode |= os.ModeDir
	} else if strings.HasPrefix(typ, "os.unix=slink") || strings.HasPrefix(typ, "os.unix=symlink") {
		// note: there is no general way to determine whether a symlink points to a dir or a file
		mode |= os.ModeSymlink
	}

	var (
		size int64
		err  error
	)

	if facts.size != "" {
		size, err = strconv.ParseInt(facts.size, 10, 64)
	} else if mode.IsDir() && facts.sizd != "" {
		size, err = strconv.ParseInt(facts.sizd, 10, 64)
	} else if typ == "file" {
		return nil, p.incompleteError(entry)
	}

	if err != nil {
		return nil, p.error(entry)
	}

	if facts.modify == "" {
		return nil, p.incompleteError(entry)
	}

	mtime, ok := p.parseModTime(facts.modify)
	if !ok {
		return nil, p.incompleteError(entry)
	}

	info := &ftpFile{
		name:  filepath.Base(filename),
		size:  size,
		mtime: mtime,
		raw:   entry,
		mode:  mode,
	}

	return info, nil
}

func (p mlstParser) error(entry string) error {
	return ftpError{err: fmt.Errorf(`failed parsing MLST entry: %s`, entry)}
}

func (p mlstParser) incompleteError(entry string) error {
	return ftpError{err: fmt.Errorf(`MLST entry incomplete: %s`, entry)}
}

// parseModTime parses file mtimes formatted as 20060102150405.
func (p *mlstParser) parseModTime(value string) (time.Time, bool) {
	if len(value) != 14 {
		return time.Time{}, false
	}
	year, err := strconv.ParseUint(value[:4], 10, 16)
	if err != nil {
		return time.Time{}, false
	}
	month, err := strconv.ParseUint(value[4:6], 10, 8)
	if err != nil {
		return time.Time{}, false
	}
	day, err := strconv.ParseUint(value[6:8], 10, 8)
	if err != nil {
		return time.Time{}, false
	}
	hour, err := strconv.ParseUint(value[8:10], 10, 8)
	if err != nil {
		return time.Time{}, false
	}
	min, err := strconv.ParseUint(value[10:12], 10, 8)
	if err != nil {
		return time.Time{}, false
	}
	sec, err := strconv.ParseUint(value[12:14], 10, 8)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(int(year), time.Month(month), int(day),
		int(hour), int(min), int(sec), 0, time.UTC), true
}
