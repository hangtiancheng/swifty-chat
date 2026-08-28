// Copyright (c) 2026 hangtiancheng
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Brand colors
	brandPurple = lipgloss.Color("99")
	dimText     = lipgloss.Color("242")
	mutedText   = lipgloss.Color("245")
	normalText  = lipgloss.Color("252")
	brightText  = lipgloss.Color("255")
	greenText   = lipgloss.Color("78")
	redText     = lipgloss.Color("203")
	yellowText  = lipgloss.Color("214")
	cyanText    = lipgloss.Color("80")

	// Banner
	bannerStyle = lipgloss.NewStyle().
			Foreground(brandPurple).
			Bold(true)

	bannerDimStyle = lipgloss.NewStyle().
			Foreground(dimText)

	// Separator line
	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("236"))

	// User prompt marker
	promptStyle = lipgloss.NewStyle().
			Foreground(cyanText).
			Bold(true)

	// AI response marker
	aiMarkerStyle = lipgloss.NewStyle().
			Foreground(brandPurple).
			Bold(true)

	// AI text
	aiTextStyle = lipgloss.NewStyle().
			Foreground(normalText).
			PaddingLeft(2)

	// Streaming text (slightly dimmer while streaming)
	streamingTextStyle = lipgloss.NewStyle().
				Foreground(normalText).
				PaddingLeft(2)

	// Tool call styles

	toolRunningStyle = lipgloss.NewStyle().
				Foreground(dimText).
				PaddingLeft(2)

	toolDoneStyle = lipgloss.NewStyle().
			Foreground(greenText).
			PaddingLeft(2)

	toolErrorStyle = lipgloss.NewStyle().
			Foreground(redText).
			PaddingLeft(2)

	toolDetailStyle = lipgloss.NewStyle().
			Foreground(dimText).
			PaddingLeft(4)

	// EditFile diff line styles: green for additions, red for deletions; context lines reuse toolDetailStyle
	diffAddStyle = lipgloss.NewStyle().
			Foreground(greenText).
			PaddingLeft(4)

	diffRemoveStyle = lipgloss.NewStyle().
			Foreground(redText).
			PaddingLeft(4)

	// Error message
	errorStyle = lipgloss.NewStyle().
			Foreground(redText).
			PaddingLeft(2)

	// Permission dialog
	permBorderStyle = lipgloss.NewStyle().
			Foreground(yellowText).
			Bold(true)

	permDimStyle = lipgloss.NewStyle().
			Foreground(dimText)

	// Status bar (bottom)
	statusBarStyle = lipgloss.NewStyle().
			Foreground(dimText)

	statusItemStyle = lipgloss.NewStyle().
			Foreground(mutedText)

	// Provider selection
	selectLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(brandPurple).
				Align(lipgloss.Center)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(cyanText).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(mutedText)
)
