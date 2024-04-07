package files

import (
	"context"
	"io/fs"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/simulot/immich-go/browser"
	"github.com/simulot/immich-go/immich"
	"github.com/simulot/immich-go/immich/metadata"
	"github.com/simulot/immich-go/logger"
)

type LocalAssetBrowser struct {
	fsyss  []fs.FS
	albums map[string]string

	log        *logger.LogAndCount[logger.UpLdAction]
	sm         immich.SupportedMedia
	whenNoDate string
}

func NewLocalFiles(ctx context.Context, log *logger.LogAndCount[logger.UpLdAction], fsyss ...fs.FS) (*LocalAssetBrowser, error) {
	return &LocalAssetBrowser{
		fsyss:      fsyss,
		albums:     map[string]string{},
		log:        log,
		whenNoDate: "FILE",
	}, nil
}

func (la *LocalAssetBrowser) Prepare(ctx context.Context) error {
	return nil
}

func (la *LocalAssetBrowser) SetSupportedMedia(sm immich.SupportedMedia) *LocalAssetBrowser {
	la.sm = sm
	return la
}

func (la *LocalAssetBrowser) SetWhenNoDate(opt string) *LocalAssetBrowser {
	la.whenNoDate = opt
	return la
}

var toOldDate = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func (la *LocalAssetBrowser) Browse(ctx context.Context) chan *browser.LocalAssetFile {
	la.log.Stage("Parsing source")
	fileChan := make(chan *browser.LocalAssetFile)
	// Browse all given FS to collect the list of files
	go func(ctx context.Context) {
		defer close(fileChan)
		for _, fsys := range la.fsyss {
			err := fs.WalkDir(fsys, ".",
				func(name string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}

					// Check if the context has been cancelled
					select {
					case <-ctx.Done():
						// If the context has been cancelled, return immediately
						return ctx.Err()
					default:
						if d.IsDir() {
							return la.handleFolder(ctx, fsys, fileChan, name)
						}
					}
					return nil
				})
			if err != nil {
				// Check if the context has been cancelled before sending the error
				select {
				case <-ctx.Done():
					// If the context has been cancelled, return immediately
					return
				case fileChan <- &browser.LocalAssetFile{
					Err: err,
				}:
				}
			}
		}
	}(ctx)

	return fileChan
}

func (la *LocalAssetBrowser) handleFolder(ctx context.Context, fsys fs.FS, fileChan chan *browser.LocalAssetFile, folder string) error {
	entries, err := fs.ReadDir(fsys, folder)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		fileName := path.Join(folder, name)
		ext := strings.ToLower(path.Ext(name))
		la.log.AddEntry(log.InfoLevel, logger.UpldDiscoveredFile, fileName)

		t := la.sm.TypeFromExt(ext)
		switch t {
		default:
			la.log.AddEntry(log.InfoLevel, logger.UpldDiscarded, fileName, "reason", "unknown extension")
			continue
		case immich.TypeIgnored:
			la.log.AddEntry(log.InfoLevel, logger.UpldDiscarded, fileName, "reason", "useless file type")
			continue
		case immich.TypeSidecar:
			la.log.AddEntry(log.InfoLevel, logger.UpldMetadata, fileName)
			continue
		case immich.TypeImage:
			la.log.AddEntry(log.InfoLevel, logger.UpldScannedImage, fileName)
		case immich.TypeVideo:
			la.log.AddEntry(log.InfoLevel, logger.UpldScannedVideo, fileName)
		}

		f := browser.LocalAssetFile{
			FSys:      fsys,
			FileName:  path.Join(folder, name),
			Title:     path.Base(name),
			FileSize:  0,
			Err:       err,
			DateTaken: metadata.TakeTimeFromName(filepath.Base(name)),
		}

		s, err := e.Info()
		if err != nil {
			f.Err = err
		} else {
			f.FileSize = int(s.Size())
			if f.DateTaken.IsZero() {
				err = la.ReadMetadataFromFile(&f)
				_ = err
				if f.DateTaken.Before(toOldDate) {
					switch la.whenNoDate {
					case "FILE":
						f.DateTaken = s.ModTime()
					case "NOW":
						f.DateTaken = time.Now()
					}
				}
			}
			la.checkSidecar(&f, entries, folder, name)
		}
		// Check if the context has been cancelled
		select {
		case <-ctx.Done():
			// If the context has been cancelled, return immediately
			return ctx.Err()
		default:
			fileChan <- &f
		}
	}
	return nil
}

func (la *LocalAssetBrowser) checkSidecar(f *browser.LocalAssetFile, entries []fs.DirEntry, dir, name string) bool {
	assetBase := la.baseNames(name)

	for _, name := range assetBase {
		xmp := name + ".[xX][mM][pP]"
		for _, e := range entries {
			m, err := path.Match(xmp, e.Name())
			if err != nil {
				panic(err)
			}
			if m {
				f.SideCar = &metadata.SideCar{
					FileName: path.Join(dir, e.Name()),
					OnFSsys:  true,
				}
				la.log.AddEntry(log.InfoLevel, logger.UpldAssociatedMetadata, "with", f.FileName)
				return true
			}
		}
	}
	return false
}

func (la *LocalAssetBrowser) baseNames(n string) []string {
	n = escapeName(n)
	names := []string{n}
	ext := path.Ext(n)
	for {
		if ext == "" {
			return names
		}
		if la.sm.TypeFromExt(ext) == "" {
			return names
		}
		n = strings.TrimSuffix(n, ext)
		names = append(names, n, n+".*")
		ext = path.Ext(n)
	}
}

func escapeName(n string) string {
	b := strings.Builder{}
	for _, c := range n {
		switch c {
		case '*', '?', '[', ']', '^':
			b.WriteRune('\\')
		case '\\':
			if runtime.GOOS != "windows" {
				b.WriteRune('\\')
			}
		}
		b.WriteRune(c)
	}
	return b.String()
}

func (la *LocalAssetBrowser) ReadMetadataFromFile(a *browser.LocalAssetFile) error {
	ext := strings.ToLower(path.Ext(a.FileName))

	// Open the file
	r, err := a.PartialSourceReader()
	if err != nil {
		return err
	}
	m, err := metadata.GetFromReader(r, ext)
	if err == nil {
		a.DateTaken = m.DateTaken
	}
	return err
}
