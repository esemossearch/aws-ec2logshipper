package main

import (
	"fmt"
	"regexp"
	"time"
)

// Parser extracts timestamps from log lines using a layout and optional regex.
type Parser struct {
	layout string
	re     *regexp.Regexp
}

// NewParser compiles the optional timestamp regex and returns a Parser for the given layout.
func NewParser(layout, re string) (*Parser, error) {
	p := &Parser{layout: layout}
	if re != "" {
		var err error
		p.re, err = regexp.Compile(re)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp regex: %w", err)
		}
	}
	return p, nil
}

// Parse tries to extract a timestamp from line; if it fails, it returns the current time.
func (p *Parser) Parse(line string) (time.Time, string) {
	if p.re != nil {
		m := p.re.FindStringSubmatch(line)
		if len(m) > 1 {
			ts, err := time.Parse(p.layout, m[1])
			if err == nil {
				return ts, line
			}
		}
	}
	return time.Now(), line
}
