package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

const (
	ErrCouldNotParseSize   = "Couldn't parse size"
	ErrCouldNotWidthHeight = "Couldn't parse width/height pair"
)

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
	Region     string
	Size       string
	Rotation   string
	Format     string
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

func main() {
	router := mux.NewRouter()

	router.HandleFunc("/{prefix}/{identifier}/{region}/{size}/{rotation}/{quality}.{format}", iiifHandler)
	router.HandleFunc("/{prefix}/{identifier}/info.json", infoHandler)

	s := &http.Server{
		Addr:    ":8080",
		Handler: cors.Default().Handler(router),
	}

	log.Printf("Listening on: %s", s.Addr)
	s.ListenAndServe()
}

func iiifHandler(w http.ResponseWriter, r *http.Request) {
	imgReq := imageReq(r)
	fmt.Fprintf(w, "iiifHandler:, %v", imgReq)
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	iReq := infoReq(r)
	fmt.Fprintf(w, "Info handler:, %v", iReq)
}

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

func imageReq(r *http.Request) ImageReq {
	vars := mux.Vars(r)
	prefix, ok := vars["prefix"]
	if !ok {
		log.Panicln("Failed to parse prefix from URL")
	}

	identifier, ok := vars["identifier"]
	if !ok {
		log.Panicln("Failed to parse identifier from URL")
	}

	region, ok := vars["region"]
	if !ok {
		log.Panicln("Failed to parse region from URL")
	}

	size, ok := vars["size"]
	if !ok {
		log.Panicln("Failed to parse size from URL")
	}

	rotation, ok := vars["rotation"]
	if !ok {
		log.Panicln("Failed to parse rotation from URL")
	}

	format, ok := vars["format"]
	if !ok {
		log.Panicln("Failed to parse format from URL")
	}

	return ImageReq{
		Req:        r,
		Prefix:     prefix,
		Identifier: identifier,
		Region:     region,
		Size:       size,
		Rotation:   rotation,
		Format:     format,
	}
}

// full 	The extracted region is not scaled, and is returned at its full size.
// w, 	The extracted region should be scaled so that its width is exactly equal to w, and the height will be a calculated value that maintains the aspect ratio of the extracted region.
// ,h 	The extracted region should be scaled so that its height is exactly equal to h, and the width will be a calculated value that maintains the aspect ratio of the extracted region.
// pct:n 	The width and height of the returned image is scaled to n% of the width and height of the extracted region. The aspect ratio of the returned image is the same as that of the extracted region.
// w,h 	The width and height of the returned image are exactly w and h. The aspect ratio of the returned image may be different than the extracted region, resulting in a distorted image.
// !w,h 	The image content is scaled for the best fit such that the resulting width and height are less than or equal to the requested width and height. The exact scaling may be determined by the service provider, based on characteristics including image quality and system performance. The dimensions of the returned image content are calculated to maintain the aspect ratio of the extracted region.

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
		} else if w != "" && h == "" {
			w64, err := strconv.ParseInt(w, 10, 64)
			width := int(w64)
			if err != nil {
				return nil, errors.New(ErrCouldNotWidthHeight)
			}
			return SizeWidth{Width: width}, nil
		} else if w == "" && h != "" {
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
