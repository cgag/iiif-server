package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
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

var validFormats = []string{"jpg", "tif", "png", "gif", "jp2", "pdf", "webp"}

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

// Region derived from original size and a region transformation
// ^ Is this comment out of date?
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

// Job .
type Job struct {
	Cmd      string
	RespChan chan []byte
}

// Context .
type Context struct {
	workerChan chan string
}

// ContextHandler .
type ContextHandler struct {
	ctx     Context
	handler func(ctx Context, w http.ResponseWriter, r *http.Request)
}

func main() {
	// TODO(cgag): need a worker pool (num cpus)
	// TODO(cgag): need memory limits as well.

	jobChan := make(chan Job)
	// TODO(cgag): more workers? or can we rely on imagemagic to use
	// use all the cores?  Is it worth breaking imagemagick's memory usage
	// heuristics?
	go func() {
		for {
			job := <-jobChan
			fmt.Printf("cmd :%s\n", job.Cmd)
		}
	}()

	router := mux.NewRouter()

	router.HandleFunc("/", helloHandler)
	// TODO(cgag): prefix is optional, need to handle that as well
	router.HandleFunc("/{prefix}/{identifier}", baseRedirect)
	router.HandleFunc(
		"/{prefix}/{identifier}/{region}/{size}/{rotation}/{quality}.{format}",
		iiifHandler)
	router.HandleFunc("/{prefix}/{identifier}/info.json", infoHandler)

	// TODO(cgag): Get port from env with a default
	s := &http.Server{
		Addr:    ":8080",
		Handler: mkLoggingHandler(router),
	}

	logrus.Infof("Listening on: %s", s.Addr)
	err := s.ListenAndServe()
	if err != nil {
		logrus.Errorf("Error starting server: %#v", s)
	}
}

func baseRedirect(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	prefix, ok := vars["prefix"]
	if !ok {
		logrus.Error("Failed to parse prefix from URL")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	identifier, ok := vars["identifier"]
	if !ok {
		logrus.Error("Failed to parse identifier from URL")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/"+prefix+"/"+identifier+"/info.json", http.StatusSeeOther)
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "iiif server")
}

func md5str(s string) string {
	x := md5.New()
	x.Write([]byte(s))
	b := x.Sum(nil)
	return hex.EncodeToString(b[:])
}

func iiifHandler(w http.ResponseWriter, r *http.Request) {

	cacheDir := "iiifCache"
	if err := os.Mkdir(cacheDir, os.FileMode(0755)); err != nil {
		if !os.IsExist(err) {
			logrus.Errorf("err creating cache dir: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	cacheFilepath := cacheDir + "/" + md5str(r.URL.String())

	// TODO(cgag): all these hardcoded /'s fuck up portability
	cachedFile, err := os.Open(cacheFilepath)
	if err != nil {
		if !os.IsNotExist(err) {
			logrus.Errorf("Unforseen problem opening cached file: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	imgReq, err := imageReq(r)
	if err != nil {
		logrus.Errorf("error with imageReq: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set("Link", "<http://iiif.io/api/image/2/level1.json>;rel=\"profile\"")
	w.Header().Set("Content-Type", mime.TypeByExtension("."+imgReq.Format))

	if cachedFile != nil {
		logrus.Info("cache hit")
		bytes, err := ioutil.ReadAll(cachedFile)
		if err != nil {
			logrus.Error("couldn't read cached file")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write(bytes)
		return
	}

	logrus.Info("cache miss")

	if !imgExists(imgReq.toPath()) {
		logrus.Infof("no such image: %s", imgReq.toPath())
		w.WriteHeader(http.StatusNotFound)
		return
	}

	args, err := imgReq.buildArgs()
	if err != nil {
		logrus.Errorf("%s", err)
		fmt.Fprintf(w, "Error with request:  %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	splitArgs := strings.Split(strings.TrimSpace(args), " ")
	out, err := exec.Command("convert", splitArgs...).Output()
	if err != nil {
		logrus.Errorf("err running convert: %s", err)
		logrus.Errorf("args were: %s", args)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// write cache
	err = ioutil.WriteFile(cacheFilepath, out, os.FileMode(0755))
	if err != nil {
		logrus.Errorf("err writing file: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Write(out)
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	wants, ok := r.Header["Accept"]
	if !ok || wants[0] == "application/json" {
		w.Header().Set("Content-Type", "application/json")
	} else if wants[0] == "application/ld+json" {
		w.Header().Set("Content-Type", "application/ld+json")
	}

	iReq := infoReq(r)
	iResp, err := iReq.infoResp()
	if err != nil {
		logrus.Errorf("err handling info request: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err = json.NewEncoder(w).Encode(iResp); err != nil {
		logrus.Errorf("Error encoding infoResponse to JSON: %#v", iResp)
	}
}

func (iReq InfoReq) infoResp() (*ImageInfo, error) {
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
	// TODO(cgag): parallelize?
	var found []string
	for _, format := range validFormats {
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

func imgStats(filepath string) (WidthHeight, error) {
	out, err :=
		exec.Command("identify", "-ping", "-format", "%w,%h", filepath).Output()
	if err != nil {
		return WidthHeight{}, err
	}
	return parseWidthHeight(string(out))
}

func (imgReq ImageReq) buildArgs() (string, error) {
	// TODO(cgag): a tempfile system for caching?

	// TODO(cgag): do this once in main...
	convertMemLimit := os.Getenv("CONVERT_MEM_LIMIT")

	stats, err := imgStats(imgReq.toPath())
	if err != nil {
		return "", err
	}

	args := ""
	switch imgReq.Region.(type) {
	case RegionFull:
		break
	case RegionExact:
		region := imgReq.Region.(RegionExact)
		args = fmt.Sprintf(
			"%s -crop %dx%d+%d+%d",
			args,
			region.Width,
			region.Height,
			region.X,
			region.Y)
	case RegionPercent:
		region := imgReq.Region.(RegionPercent)
		offsetX := int(round(float64(stats.Width) * region.X / 100.0))
		offsetY := int(round(float64(stats.Height) * region.Y / 100.0))
		args = fmt.Sprintf(
			"%s -crop %f%%x%f+%d+%d",
			args,
			region.Width,
			region.Height,
			offsetX,
			offsetY,
		)
	default:
		return "", fmt.Errorf("Unrecognized region type: %v", imgReq.Region)
	}

	switch imgReq.Size.(type) {
	case SizeFull:
		break
	case SizeHeight:
		resize := imgReq.Size.(SizeHeight)
		args = fmt.Sprintf(
			"%s -resize x%d",
			args,
			resize.Height,
		)
	case SizeWidth:
		resize := imgReq.Size.(SizeWidth)
		args = fmt.Sprintf(
			"%s -resize %dx",
			args,
			resize.Width,
		)
	case SizeExact:
		resize := imgReq.Size.(SizeExact)
		args = fmt.Sprintf(
			"%s -resize %dx%d!",
			args,
			resize.Width,
			resize.Height,
		)
	case SizePercent:
		resize := imgReq.Size.(SizePercent)
		args = fmt.Sprintf(
			"%s -resize %%%f",
			args,
			resize.Percent,
		)
	case SizeBestFit:
		resize := imgReq.Size.(SizeBestFit)
		args = fmt.Sprintf(
			"%s -resize %dx%d",
			args,
			resize.Width,
			resize.Height,
		)
	default:
		return "", fmt.Errorf("Unrecognized size type: %v\n", imgReq.Size)
	}

	switch imgReq.Rotation.(type) {
	case RotateStandard:
		rotation := imgReq.Rotation.(RotateStandard)
		if rotation.Degrees != 0 {
			args = fmt.Sprintf(
				"%s -rotate %f",
				args,
				rotation.Degrees,
			)
		}
	case RotateMirrored:
		rotation := imgReq.Rotation.(RotateMirrored)
		args = fmt.Sprintf(
			"%s -flop -rotate %f",
			args,
			rotation.Degrees,
		)
	default:
		return "", fmt.Errorf("Unrecognized rotation : %v\n", imgReq.Rotation)
	}

	switch imgReq.Quality {
	case "default":
		break
	case "color":
		break
	case "gray":
		args = fmt.Sprintf(
			"%s -colorspace Gray",
			args,
		)
	case "bitonal":
		args = fmt.Sprintf(
			"%s -colorspace Gray -type Bilevel",
			args,
		)
	default:
		return "", fmt.Errorf("Unrecognized Quality : %v\n", imgReq.Quality)
	}

	if convertMemLimit != "" {
		args = fmt.Sprintf("%s -limit memory %s", args, convertMemLimit)
	}

	// Output to stdout.
	args = fmt.Sprintf("%s %s %s:-", args, imgReq.toPath(), imgReq.Format)

	return args, nil
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
		// Drop !
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
	// TODO(cgag): document these strings.  Shoudl we accept things like
	// jpeg vs jpg?
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
func parseWidthHeight(size string) (WidthHeight, error) {
	parts := strings.Split(size, ",")
	if len(parts) != 2 {
		return WidthHeight{}, errors.New(ErrCouldNotWidthHeight)
	}

	wStr, hStr := parts[0], parts[1]

	w64, err := strconv.ParseInt(wStr, 10, 64)
	w := int(w64)
	if err != nil {
		return WidthHeight{}, errors.New(ErrCouldNotWidthHeight)
	}

	h64, err := strconv.ParseInt(hStr, 10, 64)
	h := int(h64)
	if err != nil {
		return WidthHeight{}, errors.New(ErrCouldNotWidthHeight)
	}

	return WidthHeight{
		Width:  w,
		Height: h,
	}, nil
}

// rand utils

// are you kidding me golang
func round(a float64) float64 {
	if a < 0 {
		return math.Ceil(a - 0.5)
	}
	return math.Floor(a + 0.5)
}

// logging

// HTTPLog .
type HTTPLog struct {
	http.ResponseWriter
	ip           string
	time         time.Time
	method       string
	uri          string
	protocol     string
	status       int
	bytesWritten int64
	elapsedTime  time.Duration
}

func (r *HTTPLog) Write(p []byte) (int, error) {
	written, err := r.ResponseWriter.Write(p)
	r.bytesWritten += int64(written)
	return written, err
}

// WriteHeader .
func (r *HTTPLog) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// LoggingHandler .
type LoggingHandler struct {
	handler http.Handler
}

func mkLoggingHandler(handler http.Handler) http.Handler {
	return &LoggingHandler{
		handler: handler,
	}
}

func (h *LoggingHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if colon := strings.LastIndex(clientIP, ":"); colon != -1 {
		clientIP = clientIP[:colon]
	}

	record := &HTTPLog{
		ResponseWriter: rw,
		time:           time.Time{},
		status:         http.StatusOK,
		method:         r.Method,
		uri:            r.RequestURI,
		protocol:       r.Proto,
		ip:             clientIP,
		elapsedTime:    time.Duration(0),
	}

	startTime := time.Now()
	h.handler.ServeHTTP(record, r)
	finishTime := time.Now()

	record.time = startTime.UTC()
	record.elapsedTime = finishTime.Sub(startTime)

	logrus.WithFields(logrus.Fields{
		"time":     record.time.Format(time.RFC3339),
		"status":   record.status,
		"method":   record.method,
		"protocol": r.Proto,
		"clientIP": record.ip,
		"elapsed":  record.elapsedTime,
	}).Info(record.uri)
}
