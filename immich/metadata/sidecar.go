package metadata

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"math"
	"text/template"
	"time"
)

// SideCar
type SideCar struct {
	FileName string
	OnFSsys  bool

	DateTaken time.Time
	Latitude  float64
	Longitude float64
	Elevation float64
}

func cmpFloats(a, b float64) int {
	d := a - b
	if math.Abs(d) < 1e-5 {
		return 0
	}
	if a < b {
		return -1
	}
	return 1
}

func (sc *SideCar) Open(fsys fs.FS, name string) (io.ReadCloser, error) {
	if sc.OnFSsys {
		return fsys.Open(name)
	}

	b := bytes.NewBuffer(nil)
	err := sidecarTemplate.Execute(b, sc)
	if err != nil {
		return nil, fmt.Errorf("can't generate XMP sidecar file: %w", err)
	}

	return io.NopCloser(b), nil

}

func (sc *SideCar) Bytes() ([]byte, error) {

	b := bytes.NewBuffer(nil)
	err := sidecarTemplate.Execute(b, sc)
	if err != nil {
		return nil, fmt.Errorf("can't generate XMP sidecar file: %w", err)
	}
	return b.Bytes(), nil
}

var sidecarTemplate = template.Must(template.New("xmp").Parse(`<x:xmpmeta xmlns:x='adobe:ns:meta/' x:xmptk='Image::ExifTool 12.56'>
<rdf:RDF xmlns:rdf='http://www.w3.org/1999/02/22-rdf-syntax-ns#'>
 <rdf:Description rdf:about=''
  xmlns:exif='http://ns.adobe.com/exif/1.0/'>
  <exif:ExifVersion>0232</exif:ExifVersion>
  {{if not (.DateTaken).IsZero}}<exif:DateTimeOriginal>{{((.DateTaken).Local).Format "2006-01-02T15:04:05+0000"}}</exif:DateTimeOriginal>{{end}}
  {{if and (ne .Latitude 0.0) (ne .Longitude 0.0)}}
  <exif:GPSAltitude>{{.Elevation}}</exif:GPSAltitude>
  <exif:GPSLatitude>{{.Latitude}}</exif:GPSLatitude>
  <exif:GPSLongitude>{{.Longitude}}</exif:GPSLongitude>  
  <exif:GPSTimeStamp>{{.DateTaken.Format "2006-01-02T15:04:05+0000"}}</exif:GPSTimeStamp>
  {{end}}
 </rdf:Description>
</rdf:RDF>
</x:xmpmeta>`))