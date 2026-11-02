package doctype

import (
	"encoding/hex"
	"fmt"
)

type PageSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type PageRef struct {
	Source string
	Number int
	Width  int
	Height int
}

type PageDocument struct {
	Source string           `json:"source"`
	Pages  map[int]PageSize `json:"pages"`
}

func (size PageSize) Validate() error {
	if size.Width <= 0 || size.Height <= 0 || size.Width > 40_000_000 || size.Height > 40_000_000 || int64(size.Width)*int64(size.Height) > 40_000_000 {
		return fmt.Errorf("invalid PDF page dimensions")
	}
	return nil
}

func (ref PageRef) Validate() error {
	digest, err := hex.DecodeString(ref.Source)
	if err != nil || len(digest) != 32 || ref.Number < 0 {
		return fmt.Errorf("invalid PDF page reference")
	}
	return (PageSize{Width: ref.Width, Height: ref.Height}).Validate()
}

func (document PageDocument) Validate() error {
	digest, err := hex.DecodeString(document.Source)
	if err != nil || len(digest) != 32 {
		return fmt.Errorf("invalid PDF source digest")
	}
	for number, size := range document.Pages {
		if number < 0 {
			return fmt.Errorf("invalid PDF page number")
		}
		if err := size.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (document PageDocument) Page(number int) (PageRef, bool) {
	size, ok := document.Pages[number]
	return PageRef{Source: document.Source, Number: number, Width: size.Width, Height: size.Height}, ok
}
