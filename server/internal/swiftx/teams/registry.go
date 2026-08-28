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

package teams

import "sync"

// NameRegistry is the process-wide global name registry, maintaining
// member name → agentID mappings. SendMessage uses it to resolve the recipient
// name given by the user/LLM into a delivery identifier.
type NameRegistry struct {
	mu    sync.Mutex
	names map[string]string // name -> agentID
}

var (
	nameRegistryOnce     sync.Once
	nameRegistryInstance *NameRegistry
)

// GetNameRegistry returns the global singleton registry.
func GetNameRegistry() *NameRegistry {
	nameRegistryOnce.Do(func() {
		nameRegistryInstance = &NameRegistry{names: make(map[string]string)}
	})
	return nameRegistryInstance
}

// Register records a name → agentID mapping.
func (r *NameRegistry) Register(name, agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names[name] = agentID
}

// Resolve resolves a name or ID into an agentID: it looks up the name first;
// if the input is itself an ID, it is returned as-is; otherwise an empty
// string is returned.
func (r *NameRegistry) Resolve(nameOrID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.names[nameOrID]; ok {
		return id
	}
	for _, id := range r.names {
		if id == nameOrID {
			return nameOrID
		}
	}
	return ""
}

// Unregister removes the mapping for a name.
func (r *NameRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.names, name)
}

// Clear removes all mappings; mainly used for test isolation.
func (r *NameRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = make(map[string]string)
}
