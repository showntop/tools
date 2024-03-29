// Copyright (c) 2014 The gomqtt Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tools

import (
	"fmt"
	"strings"
	"sync"
)

type node struct {
	children map[string]*node
	values   []interface{}
}

func newNode() *node {
	return &node{
		children: make(map[string]*node),
		values:   make([]interface{}, 0),
	}
}

func (n *node) removeValue(value interface{}) {
	for i, v := range n.values {
		if v == value {
			// remove without preserving order
			n.values[i] = n.values[len(n.values)-1]
			n.values = n.values[:len(n.values)-1]
			break
		}
	}
}

func (n *node) clearValues() {
	n.values = make([]interface{}, 0)
}

func (n *node) string(i int) string {
	str := ""

	if i != 0 {
		str = fmt.Sprintf("%d", len(n.values))
	}

	for key, node := range n.children {
		str += fmt.Sprintf("\n| %s'%s' => %s", strings.Repeat(" ", i*2), key, node.string(i+1))
	}

	return str
}

// The Tree implements a thread-safe topic tree.
type Tree struct {
	// The separator character. Default: "/"
	Separator string

	// The single level wildcard character. Default: "+"
	WildcardOne string

	// The multi level wildcard character. Default "#"
	WildcardSome string

	root  *node
	mutex sync.RWMutex
}

// NewTree returns a new Tree.
func NewTree() *Tree {
	return &Tree{
		Separator:    "/",
		WildcardOne:  "+",
		WildcardSome: "#",

		root: newNode(),
	}
}

// Add registers the value for the supplied topic. This function will
// automatically grow the tree. If value already exists for the given topic it
// will not be added again.
func (t *Tree) Add(topic string, value interface{}) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.add(value, 0, strings.Split(topic, t.Separator), t.root)
}

func (t *Tree) add(value interface{}, i int, segments []string, node *node) {
	// add value to leaf
	if i == len(segments) {
		for _, v := range node.values {
			if v == value {
				return
			}
		}

		node.values = append(node.values, value)
		return
	}

	segment := segments[i]
	child, ok := node.children[segment]

	// create missing node
	if !ok {
		child = newNode()
		node.children[segment] = child
	}

	t.add(value, i+1, segments, child)
}

// Set sets the supplied value as the only value for the supplied topic. This
// function will automatically grow the tree.
func (t *Tree) Set(topic string, value interface{}) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.set(value, 0, strings.Split(topic, t.Separator), t.root)
}

func (t *Tree) set(value interface{}, i int, segments []string, node *node) {
	// set value on leaf
	if i == len(segments) {
		node.values = []interface{}{value}
		return
	}

	segment := segments[i]
	child, ok := node.children[segment]

	// create missing node
	if !ok {
		child = newNode()
		node.children[segment] = child
	}

	t.set(value, i+1, segments, child)
}

// Remove unregisters the value from the supplied topic. This function will
// automatically shrink the tree.
func (t *Tree) Remove(topic string, value interface{}) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.remove(value, 0, strings.Split(topic, t.Separator), t.root)
}

// Empty will unregister all values from the supplied topic. This function will
// automatically shrink the tree.
func (t *Tree) Empty(topic string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.remove(nil, 0, strings.Split(topic, t.Separator), t.root)
}

func (t *Tree) remove(value interface{}, i int, segments []string, node *node) bool {
	// clear or remove value from leaf node
	if i == len(segments) {
		if value == nil {
			node.clearValues()
		} else {
			node.removeValue(value)
		}

		return len(node.values) == 0 && len(node.children) == 0
	}

	segment := segments[i]
	child, ok := node.children[segment]

	// node not found
	if !ok {
		return false
	}

	if t.remove(value, i+1, segments, child) {
		delete(node.children, segment)
	}

	return len(node.values) == 0 && len(node.children) == 0
}

// Clear will unregister the supplied value from all topics. This function will
// automatically shrink the tree.
func (t *Tree) Clear(value interface{}) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.clear(value, t.root)
}

func (t *Tree) clear(value interface{}, node *node) bool {
	node.removeValue(value)

	// remove value from all nodes
	for segment, child := range node.children {
		if t.clear(value, child) {
			delete(node.children, segment)
		}
	}

	return len(node.values) == 0 && len(node.children) == 0
}

// Match will return a set of values from topics that match the supplied topic.
// The result set will be cleared from duplicate values.
//
// Note: In contrast to Search, Match does not respect wildcards in the query but
// in the stored tree.
func (t *Tree) Match(topic string) []interface{} {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	segments := strings.Split(topic, t.Separator)
	values := t.match(make([]interface{}, 0), 0, segments, t.root)

	return t.clean(values)
}

func (t *Tree) match(result []interface{}, i int, segments []string, node *node) []interface{} {
	// add all values to the result set that match multiple levels
	if child, ok := node.children[t.WildcardSome]; ok {
		result = append(result, child.values...)
	}

	// when finished add all values to the result set
	if i == len(segments) {
		return append(result, node.values...)
	}

	// advance children that match a single level
	if child, ok := node.children[t.WildcardOne]; ok {
		result = t.match(result, i+1, segments, child)
	}

	segment := segments[i]

	// match segments and get children
	if segment != t.WildcardOne && segment != t.WildcardSome {
		if child, ok := node.children[segment]; ok {
			result = t.match(result, i+1, segments, child)
		}
	}

	return result
}

// Search will return a set of values from topics that match the supplied topic.
// The result set will be cleared from duplicate values.
//
// Note: In contrast to Match, Search respects wildcards in the query but not in
// the stored tree.
func (t *Tree) Search(topic string) []interface{} {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	segments := strings.Split(topic, t.Separator)
	values := t.search(make([]interface{}, 0), 0, segments, t.root)

	return t.clean(values)
}

func (t *Tree) search(result []interface{}, i int, segments []string, node *node) []interface{} {
	// when finished add all values to the result set
	if i == len(segments) {
		return append(result, node.values...)
	}

	// get segment
	segment := segments[i]

	// add all current and further values
	if segment == t.WildcardSome {
		result = append(result, node.values...)

		for _, child := range node.children {
			result = t.search(result, i, segments, child)
		}
	}

	// add all current values and continue
	if segment == t.WildcardOne {
		result = append(result, node.values...)

		for _, child := range node.children {
			result = t.search(result, i+1, segments, child)
		}
	}

	// match segments and get children
	if segment != t.WildcardOne && segment != t.WildcardSome {
		if child, ok := node.children[segment]; ok {
			result = t.search(result, i+1, segments, child)
		}
	}

	return result
}

// clean will remove remove duplicates
func (t *Tree) clean(values []interface{}) []interface{} {
	result := make([]interface{}, 0, len(values))
	seen := make(map[interface{}]bool, len(values))

	for _, v := range values {
		if _, ok := seen[v]; !ok {
			result = append(result, v)
			seen[v] = true
		}
	}

	return result
}

// All will return all stored values in the tree.
func (t *Tree) All() []interface{} {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.clean(t.all(make([]interface{}, 0), t.root))
}

func (t *Tree) all(result []interface{}, node *node) []interface{} {
	// add children to results
	for _, child := range node.children {
		result = t.all(result, child)
	}

	// add current node to results
	return append(result, node.values...)
}

// Reset will completely clear the tree.
func (t *Tree) Reset() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.root = newNode()
}

// String will return a string representation of the tree.
func (t *Tree) String() string {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return fmt.Sprintf("topic.Tree:%s", t.root.string(0))
}
