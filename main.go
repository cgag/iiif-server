package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const (
	// ErrCouldNotParseSize .
	ErrCouldNotParseSize = "Couldn't parse size"
	// ErrCouldNotParseRotation .
	ErrCouldNotParseRotation = "Couldn't parse rotation"
	// ErrCouldNotParseRegion .
	ErrCouldNotParseRegion = "Couldn't parse region"
	// ErrCouldNotWidthHeight .
	ErrCouldNotWidthHeight = "Couldn't parse width/height pair"
	// ErrDegreesOutOfRange .
	ErrDegreesOutOfRange = "Invalid rotation: degrees out of range (0, 360)"
	// ErrInvalidQuality .
	ErrInvalidQuality = "Invalid quality"
	// ErrInvalidFormat .
	ErrInvalidFormat = "Invalid format"
)

// WidthHeight .
type WidthHeight struct {
	Width  int
	Height int
}

// InfoReq represents a request for metadata about an image
type InfoReq struct {
	Req        *http.Request
	Prefix     string
	Identifier string
}

// ImageReq represents a request for IIIF image
type ImageReq struct {
	Req        *http.Request
	Prefix     string
	Identifier string
	Region     interface{}
	Size       interface{}
	Rotation   interface{}
	Format     string
	Quality    string
}

func (imgReq ImageReq) toPath() string {
	return "images/" + imgReq.Identifier + "." + imgReq.Format
}

// SizeFull .
type SizeFull struct {
}

// SizeWidth .
type SizeWidth struct {
	Width int
}

// SizeHeight .
type SizeHeight struct {
	Height int
}

// SizePercent .
type SizePercent struct {
	Percent float64
}

// SizeExact .
type SizeExact struct {
	Width  int
	Height int
}

// SizeBestFit .
type SizeBestFit struct {
	Width  int
	Height int
}

// RegionFull .
type RegionFull struct {
}

// derviced from original size and a region transformation
type Region struct {
	X      int
	Y      int
	Width  int
	Height int
}

// RegionExact .
type RegionExact struct {
	X      int
	Y      int
	Width  int
	Height int
}

// RegionPercent .
type RegionPercent struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// RotateStandard .
type RotateStandard struct {
	Degrees float64
}

// RotateMirrored .
type RotateMirrored struct {
	Degrees float64
}

// ImageInfo represents an Image Information response
type ImageInfo struct {
	Context  string        `json:"@context"`
	ID       string        `json:"@id"`
	Protocol string        `json:"protocol"`
	Width    int           `json:"width"`
	Height   int           `json:"height"`
	Profile  []interface{} `json:"profile"`
}

// Profile .
type Profile struct {
	Context *string  `json:"@context"`
	ID      *string  `json:"@id"`
	Type    *string  `json:"@type"`
	Formats []string `json:"formats"`
}

func main() {
	router := mux.NewRouter()

	router.HandleFunc("/", helloHandler)
	// TODO(cgag): prefix is optional, need to handle that as well
	router.HandleFunc("/{prefix}/{identifier}/{region}/{size}/{rotation}/{quality}.{format}", iiifHandler)
	router.HandleFunc("/{prefix}/{identifier}/info.json", infoHandler)

	// TODO(cgag): request timing
	s := &http.Server{
		Addr:    ":8080",
		Handler: handlers.CombinedLoggingHandler(os.Stdout, handlers.CORS()(router)),
	}

	log.Printf("Listening on: %s", s.Addr)
	s.ListenAndServe()
}

// are you kidding me golang
func round(a float64) float64 {
	if a < 0 {
		return math.Ceil(a - 0.5)
	}
	return math.Floor(a + 0.5)
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "iiif hello 2")
}

func iiifHandler(w http.ResponseWriter, r *http.Request) {
	imgReq, err := imageReq(r)
	if err != nil {
		fmt.Printf("err: %s\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !imgExists(imgReq.toPath()) {
		fmt.Printf("no such image: %s\n", imgReq.toPath())
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// formats, err := getFormats(imgReq.Identifier)
	// if err != nil {
	// 	w.WriteHeader(http.StatusInternalServerError)
	// 	return
	// }

	stats, err := imgStats(imgReq.toPath())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO(cgag): a tempfile system for caching?
	cmd := "convert"

	switch imgReq.Region.(type) {
	case RegionFull:
		break
	case RegionExact:
		region := imgReq.Region.(RegionExact)
		cmd = fmt.Sprintf(
			"%s -crop %dx%d+%d+%d",
			cmd,
			region.Width,
			region.Height,
			region.X,
			region.Y)
	case RegionPercent:
		region := imgReq.Region.(RegionPercent)
		offsetX := int(round(float64(stats.Width) * region.X / 100.0))
		offsetY := int(round(float64(stats.Height) * region.Y / 100.0))
		cmd = fmt.Sprintf(
			"%s -crop %f%%x%f+%d+%d",
			cmd,
			region.Width,
			region.Height,
			offsetX,
			offsetY,
		)
	default:
		fmt.Fprintf(w, "Unrecognized region type: %v\n", imgReq.Region)
	}

	switch imgReq.Size.(type) {
	case SizeFull:
		break
	case SizeHeight:
		fmt.Println("resizing height")
		resize := imgReq.Size.(SizeHeight)
		cmd = fmt.Sprintf(
			"%s -resize x%d",
			cmd,
			resize.Height,
		)
	case SizeWidth:
		fmt.Println("size width")
		resize := imgReq.Size.(SizeWidth)
		cmd = fmt.Sprintf(
			"%s -resize %dx",
			cmd,
			resize.Width,
		)
	case SizeExact:
		fmt.Println("resizing exact")
		resize := imgReq.Size.(SizeExact)
		cmd = fmt.Sprintf(
			"%s -resize %dx%d!",
			cmd,
			resize.Width,
			resize.Height,
		)
	case SizePercent:
		resize := imgReq.Size.(SizePercent)
		cmd = fmt.Sprintf(
			"%s -resize %%%f",
			cmd,
			resize.Percent,
		)
	case SizeBestFit:
		resize := imgReq.Size.(SizeBestFit)
		cmd = fmt.Sprintf(
			"%s -resize %dx%d",
			cmd,
			resize.Width,
			resize.Height,
		)
	default:
		fmt.Fprintf(w, "Unrecognized size type: %v\n", imgReq.Size)
	}

	switch imgReq.Rotation.(type) {
	case RotateStandard:
		// fmt.Fprintf(w, "RotateStandard: %v\n", imgReq.Rotation)
		break
	case RotateMirrored:
		fmt.Fprintf(w, "RotateMirrored: %v\n", imgReq.Rotation)
		w.WriteHeader(http.StatusBadRequest)
		return
	default:
		fmt.Fprintf(w, "Unrecognized rotation : %v\n", imgReq.Rotation)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Output to stdout.
	cmd = fmt.Sprintf("%s %s %s:-", cmd, imgReq.toPath(), imgReq.Format)

	fmt.Printf("\n\ncommand: %s\n\n", cmd)

	cmdParts := strings.Split(cmd, " ")
	name, args := cmdParts[0], cmdParts[1:]
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		fmt.Printf("err: %s\n\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fmt.Printf("mime type: %s\n", mime.TypeByExtension("."+imgReq.Format))
	w.Header().Set("Content-Type", mime.TypeByExtension("."+imgReq.Format))
	w.Write(out)
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	iReq := infoReq(r)
	iResp, err := infoResp(iReq)
	if err != nil {
		fmt.Printf("err: %s", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(iResp)
}

func infoResp(iReq InfoReq) (*ImageInfo, error) {
	// TODO(cgag): where should "test_images" have come from? Is using
	// Prefix reasonable?
	formats, err := getFormats(iReq.Identifier)
	if err != nil {
		return nil, err
	}

	profiles := []interface{}{"http://iiif.io/api/image/2/level2.json"}
	profiles = append(profiles, Profile{
		Formats: formats,
	})

	stats, err := imgStats("./images/" + iReq.Identifier + "." + formats[0])
	if err != nil {
		return nil, err
	}

	return &ImageInfo{
		Context:  "http://iiif.io/api/image/2/context.json",
		ID:       "https://iiif.curtis.io" + "/" + iReq.Prefix + "/" + iReq.Identifier,
		Protocol: "http://iiif.io/api/image",
		Profile:  profiles,
		Width:    stats.Width,
		Height:   stats.Height,
	}, nil
}

func getFormats(identifier string) ([]string, error) {
	possibleFormats := []string{"png", "jpg", "tiff", "jp", "jp2"}

	// TODO(cgag): parallelize?
	var found []string
	for _, format := range possibleFormats {
		// TODO(cgag): parallel?
		path := "./images/" + identifier + "." + format
		if _, err := os.Stat(path); err == nil {
			found = append(found, format)
		}
	}
	return found, nil
}

func imgExists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return false
	}
	return true
}

func imgStats(filepath string) (*WidthHeight, error) {
	out, err :=
		exec.Command("identify", "-ping", "-format", "%w,%h", filepath).Output()
	if err != nil {
		return nil, err
	}

	return parseWidthHeight(string(out))
}

//////////////
// Parsing  //
//////////////
func infoReq(r *http.Request) InfoReq {
	vars := mux.Vars(r)
	prefix, ok := vars["prefix"]
	if !ok {
		log.Panicln("Failed to parse prefix from URL")
	}

	identifier, ok := vars["identifier"]
	if !ok {
		log.Panicln("Failed to parse identifier from URL")
	}

	return InfoReq{
		Req:        r,
		Prefix:     prefix,
		Identifier: identifier,
	}
}

func imageReq(r *http.Request) (ImageReq, error) {
	vars := mux.Vars(r)
	prefix, ok := vars["prefix"]
	if !ok {
		return ImageReq{}, errors.New("Failed to parse prefix from URL")
	}

	rawIdentifier, ok := vars["identifier"]
	if !ok {
		return ImageReq{}, errors.New("Failed to parse identifier from URL")
	}
	identifier, err := parseIdentifier(rawIdentifier)
	if err != nil {
		return ImageReq{}, errors.New("Failed to parse identifier (parseIdentifier)")
	}

	rawRegion, ok := vars["region"]
	if !ok {
		return ImageReq{}, errors.New("Failed to parse region from URL")
	}
	region, err := parseRegion(rawRegion)
	if err != nil {
		return ImageReq{}, errors.New("Failed to parse region (parseRegion)")
	}

	rawSize, ok := vars["size"]
	if !ok {
		return ImageReq{}, errors.New("Failed to parse size from URL")
	}
	size, err := parseSize(rawSize)
	if err != nil {
		return ImageReq{}, errors.New("Failed to parse size (parseSize)")
	}

	rawRotation, ok := vars["rotation"]
	if !ok {
		return ImageReq{}, errors.New("Failed to parse rotation from URL")
	}
	rotation, err := parseRotation(rawRotation)
	if err != nil {
		return ImageReq{}, err
	}

	rawQuality, ok := vars["quality"]
	if !ok {
		return ImageReq{}, errors.New("Failed to parse quality from URL")
	}
	quality, err := parseQuality(rawQuality)
	if err != nil {
		return ImageReq{}, err
	}

	rawFormat, ok := vars["format"]
	if !ok {
		return ImageReq{}, errors.New("Failed to parse format from URL")
	}
	format, err := parseFormat(rawFormat)
	if err != nil {
		return ImageReq{}, err
	}

	return ImageReq{
		Req:        r,
		Prefix:     prefix,
		Identifier: *identifier,
		Region:     region,
		Size:       size,
		Rotation:   rotation,
		Quality:    *quality,
		Format:     *format,
	}, nil
}

func parseIdentifier(identifier string) (*string, error) {
	if strings.ContainsAny(identifier, "/?#[]@%") {
		return nil, errors.New("identifier contains illegal characters")
	}
	return &identifier, nil
}

func parseRegion(region string) (interface{}, error) {
	if region == "full" {
		return RegionFull{}, nil
	}

	if strings.HasPrefix(region, "pct:") {
		parts := strings.Split(region[4:], ",")
		floats := make([]float64, 4)
		for i, p := range parts {
			if p == "" {
				return nil, errors.New(ErrCouldNotParseRegion)
			}
			pFloat, err := strconv.ParseFloat(p, 64)
			if err != nil {
				return nil, errors.New(ErrCouldNotParseRegion)
			}
			floats[i] = pFloat
		}

		return RegionPercent{
			X:      floats[0],
			Y:      floats[1],
			Width:  floats[2],
			Height: floats[3],
		}, nil
	}

	if strings.Contains(region, ",") {
		parts := strings.Split(region, ",")
		ints := make([]int, 4)
		for i, p := range parts {
			if p == "" {
				return nil, errors.New(ErrCouldNotParseRegion)
			}
			pInt, err := strconv.ParseInt(p, 10, 64)
			if err != nil {
				return nil, errors.New(ErrCouldNotParseRegion)
			}
			ints[i] = int(pInt)
		}

		return RegionExact{
			X:      ints[0],
			Y:      ints[1],
			Width:  ints[2],
			Height: ints[3],
		}, nil
	}

	return nil, errors.New(ErrCouldNotParseRegion)
}

// Returns one of the SizeXXX structs from above.
func parseSize(size string) (interface{}, error) {
	// Full
	if size == "full" {
		return SizeFull{}, nil
	}

	// Percent
	if strings.HasPrefix(size, "pct:") {
		parts := strings.Split(size, ":")
		if parts[1] == "" {
			return nil, errors.New(ErrCouldNotParseSize)
		}

		pct, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, errors.New(ErrCouldNotParseSize)
		}

		return SizePercent{Percent: pct}, nil
	}

	// BestFit (Dealers Choice)
	if strings.HasPrefix(size, "!") {
		// Drop ! and parse
		wh, err := parseWidthHeight(size[1:])
		if err != nil {
			return nil, err
		}
		return SizeBestFit{
			Width:  wh.Width,
			Height: wh.Height,
		}, nil
	}

	// Width/Height/Both
	if strings.Contains(size, ",") {
		parts := strings.Split(size, ",")
		w, h := parts[0], parts[1]

		if w != "" && h != "" {
			wh, err := parseWidthHeight(size)
			if err != nil {
				return nil, err
			}
			return SizeExact{
				Width:  wh.Width,
				Height: wh.Height,
			}, nil
		}
		if w != "" && h == "" {
			w64, err := strconv.ParseInt(w, 10, 64)
			width := int(w64)
			if err != nil {
				return nil, errors.New(ErrCouldNotWidthHeight)
			}
			return SizeWidth{Width: width}, nil
		}
		if w == "" && h != "" {
			h64, err := strconv.ParseInt(h, 10, 64)
			height := int(h64)
			if err != nil {
				return nil, errors.New(ErrCouldNotWidthHeight)
			}
			return SizeHeight{Height: height}, nil
		}
	}

	return nil, errors.New(ErrCouldNotParseSize)
}

func parseRotation(rotation string) (interface{}, error) {
	var r string
	var rType string
	if strings.HasPrefix(rotation, "!") {
		r = rotation[1:]
		rType = "mirrored"
	} else {
		r = rotation
		rType = "standard"
	}

	degrees, err := strconv.ParseFloat(r, 64)
	if err != nil {
		return nil, errors.New(ErrCouldNotParseRotation)
	}

	if degrees > 360 || degrees < 0 {
		return nil, errors.New(ErrDegreesOutOfRange)
	}

	if rType == "mirrored" {
		return RotateMirrored{Degrees: degrees}, nil
	}
	return RotateStandard{Degrees: degrees}, nil
}

func parseQuality(quality string) (*string, error) {
	found := false
	for _, valid := range []string{"color", "gray", "bitonal", "default"} {
		if quality == valid {
			found = true
		}
	}
	if found == false {
		return nil, errors.New(ErrInvalidQuality)
	}
	return &quality, nil
}

func parseFormat(format string) (*string, error) {
	validFormats := []string{"jpg", "tif", "png", "gif", "jp2", "pdf", "webp"}
	found := false
	for _, valid := range validFormats {
		if format == valid {
			found = true
		}
	}
	if found == false {
		return nil, errors.New(ErrInvalidFormat)
	}
	return &format, nil
}

// parse width/height of form "w,h"
func parseWidthHeight(size string) (*WidthHeight, error) {
	parts := strings.Split(size, ",")
	if len(parts) != 2 {
		return nil, errors.New(ErrCouldNotWidthHeight)
	}

	wStr, hStr := parts[0], parts[1]

	w64, err := strconv.ParseInt(wStr, 10, 64)
	w := int(w64)
	if err != nil {
		return nil, errors.New(ErrCouldNotWidthHeight)
	}

	h64, err := strconv.ParseInt(hStr, 10, 64)
	h := int(h64)
	if err != nil {
		return nil, errors.New(ErrCouldNotWidthHeight)
	}

	return &WidthHeight{
		Width:  w,
		Height: h,
	}, nil
}
