//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Package io — PDF writer (pandoc + xelatex).
//
// WritePDF follows the Python DocGenerator path: markdown content is
// rendered by pandoc with xelatex. This keeps PDF body rendering aligned
// with agent/component/docs_generator.py instead of drawing low-level
// text operators by hand.
package io

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// PDFOptions is the public contract for the PDF writer.
type PDFOptions struct {
	FontSize       int
	HeaderText     string
	FooterText     string
	WatermarkText  string
	AddPageNumbers bool
	AddTimestamp   bool
	FontFamily     string
}

var ErrPDFFontNotConfigured = errors.New("PDF font not configured: install a xelatex-visible CJK font such as Noto Sans CJK SC")

// WritePDF renders the content to a PDF byte stream.
//
// Layout:
//
// The body rendering intentionally goes through pandoc/xelatex to match
// the Python implementation. Header/footer/watermark overlays are handled
// by LaTeX declarations when requested.
func WritePDF(content string, opts PDFOptions) ([]byte, error) {
	if opts.FontSize <= 0 {
		opts.FontSize = 12
	}
	if opts.FontFamily == "" {
		opts.FontFamily = "Noto Sans CJK SC"
	}
	if _, err := exec.LookPath("pandoc"); err != nil {
		return nil, fmt.Errorf("PDF: pandoc not found: %w", err)
	}
	if _, err := exec.LookPath("xelatex"); err != nil {
		return nil, fmt.Errorf("PDF: xelatex not found: %w", err)
	}

	dir, err := os.MkdirTemp("", "ragflow-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("PDF: tmpdir: %w", err)
	}
	defer os.RemoveAll(dir)

	headerPath := dir + "/header.tex"
	if err := os.WriteFile(headerPath, []byte(buildPDFHeader(opts)), 0o600); err != nil {
		return nil, fmt.Errorf("PDF: write header tex: %w", err)
	}
	outPath := dir + "/out.pdf"
	args := []string{
		"--standalone",
		"--from=markdown",
		"--to=pdf",
		"--pdf-engine=xelatex",
		"--include-in-header=" + headerPath,
		"-V", "mainfont=" + opts.FontFamily,
		"-V", "CJKmainfont=" + opts.FontFamily,
		"-o", outPath,
	}
	cmd := exec.Command("pandoc", args...)
	cmd.Stdin = strings.NewReader(content)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("PDF: pandoc: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return os.ReadFile(outPath)
}

func buildPDFHeader(opts PDFOptions) string {
	fontSize := opts.FontSize
	leading := float64(fontSize) * 1.2
	h1 := fontSize + 6
	h2 := fontSize + 4
	h3 := fontSize + 2
	lines := []string{
		`\usepackage{xeCJK}`,
		`\usepackage{fancyhdr}`,
		`\usepackage{eso-pic}`,
		`\usepackage{graphicx}`,
		`\usepackage{xcolor}`,
		`\makeatletter`,
		fmt.Sprintf(`\renewcommand\normalsize{\@setfontsize\normalsize{%dpt}{%.1fpt}}`, fontSize, leading),
		`\normalsize`,
		fmt.Sprintf(`\renewcommand\section{\@startsection{section}{1}{\z@}{-3.5ex \@plus -1ex \@minus -.2ex}{2.3ex \@plus .2ex}{\normalfont\fontsize{%dpt}{%.1fpt}\selectfont\bfseries}}`, h1, float64(h1)*1.2),
		fmt.Sprintf(`\renewcommand\subsection{\@startsection{subsection}{2}{\z@}{-3.25ex\@plus -1ex \@minus -.2ex}{1.5ex \@plus .2ex}{\normalfont\fontsize{%dpt}{%.1fpt}\selectfont\bfseries}}`, h2, float64(h2)*1.2),
		fmt.Sprintf(`\renewcommand\subsubsection{\@startsection{subsubsection}{3}{\z@}{-3.25ex\@plus -1ex \@minus -.2ex}{1.5ex \@plus .2ex}{\normalfont\fontsize{%dpt}{%.1fpt}\selectfont\bfseries}}`, h3, float64(h3)*1.2),
		`\makeatother`,
	}
	if opts.HeaderText != "" || opts.FooterText != "" || opts.AddTimestamp || opts.AddPageNumbers {
		lines = append(lines,
			`\pagestyle{fancy}`,
			`\fancyhf{}`,
		)
		if opts.HeaderText != "" {
			lines = append(lines, fmt.Sprintf(`\fancyhead[L]{%s}`, escapeLatex(opts.HeaderText)))
		}
		footer := []string{}
		if opts.FooterText != "" {
			footer = append(footer, escapeLatex(opts.FooterText))
		}
		if opts.AddTimestamp {
			footer = append(footer, escapeLatex("Generated: "+time.Now().Format("2006-01-02 15:04:05")))
		}
		if opts.AddPageNumbers {
			footer = append(footer, `Page \thepage`)
		}
		if len(footer) > 0 {
			lines = append(lines, fmt.Sprintf(`\fancyfoot[C]{%s}`, strings.Join(footer, ` \textbar{} `)))
		}
	}
	if opts.WatermarkText != "" {
		lines = append(lines, fmt.Sprintf(`\AddToShipoutPictureBG{\AtPageCenter{\makebox[0pt]{\rotatebox{45}{\textcolor[gray]{0.85}{\fontsize{48}{58}\selectfont %s}}}}}`, escapeLatex(opts.WatermarkText)))
	}
	return strings.Join(lines, "\n")
}

func escapeLatex(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\textbackslash{}`,
		`{`, `\{`,
		`}`, `\}`,
		`$`, `\$`,
		`&`, `\&`,
		`#`, `\#`,
		`_`, `\_`,
		`%`, `\%`,
		`~`, `\textasciitilde{}`,
		`^`, `\textasciicircum{}`,
	)
	return replacer.Replace(s)
}

// splitLines is a conservative wrapper that splits on \n and
// preserves blank lines as empty strings.
func splitLines(content string) []string {
	if content == "" {
		return []string{""}
	}
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, "\r")
	}
	return lines
}

// isFontNotFound reports whether the gopdf error indicates a missing
// TTF registration. We match the substrings that have been stable
// across recent gopdf versions.
func isFontNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "font") && (strings.Contains(s, "not") || strings.Contains(s, "no such") || strings.Contains(s, "undefined"))
}
