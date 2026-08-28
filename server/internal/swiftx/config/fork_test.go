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

package config

import "testing"

// enable_fork defaults to enabled, and an explicit false in the config must
// actually turn it off. Storing it as a plain bool would make "not set" and
// "set to false" share the same zero value, so the latter could never disable it.
func TestForkEnabledDefaultsOn(t *testing.T) {
	cfg := &AppConfig{}
	if !cfg.ForkEnabled() {
		t.Fatal("fork should be enabled when enable_fork is not set in the config")
	}
}

func TestForkEnabledExplicitFalse(t *testing.T) {
	off := false
	cfg := &AppConfig{EnableFork: &off}
	if cfg.ForkEnabled() {
		t.Fatal("fork should be disabled when the config sets enable_fork: false")
	}

	on := true
	cfg.EnableFork = &on
	if !cfg.ForkEnabled() {
		t.Fatal("fork should be enabled when the config sets enable_fork: true")
	}
}

// A later-loaded config only overrides when it explicitly sets enable_fork;
// otherwise it inherits the value from the previous layer.
func TestMergeConfigFork(t *testing.T) {
	off := false

	base := &AppConfig{EnableFork: &off}
	merged := mergeConfig(base, &AppConfig{})
	if merged.ForkEnabled() {
		t.Fatal("a later layer without enable_fork must not override the previous layer's false")
	}

	base2 := &AppConfig{}
	merged2 := mergeConfig(base2, &AppConfig{EnableFork: &off})
	if merged2.ForkEnabled() {
		t.Fatal("a later layer setting enable_fork: false should override the default value")
	}
}
