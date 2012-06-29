package gotomic

import (
	"fmt" 
	"sync/atomic"
	"bytes"
	"unsafe"
)

type hit struct {
	left *node
	node *node
	right *node
}
func (self *hit) String() string {
	return fmt.Sprintf("&hit{%v,%v,%v}", self.left.val(), self.node.val(), self.right.val())
}

type Comparable interface {
	Compare(Thing) int
}

type Thing interface{}

type List struct {
	*node
	size int64
}
func NewList() *List {
	return &List{&node{}, 0}
}
func (self *List) Push(t Thing) {
	self.node.add(t)
	atomic.AddInt64(&self.size, 1)
}
func (self *List) Pop() Thing {
	rval := self.node.remove()
	atomic.AddInt64(&self.size, -1)
	return rval
}
func (self *List) String() string {
	return fmt.Sprint(self.ToSlice())
}
func (self *List) ToSlice() []Thing {
	return self.node.next().ToSlice()
}
func (self *List) Search(c Comparable) Thing {
	if hit := self.node.search(c); hit.node != nil {
		return hit.node.val()
	}
	return nil
}
func (self *List) Size() int {
	return int(atomic.LoadInt64(&self.size))
}
func (self *List) Inject(c Comparable) {
	self.node.inject(c)
	atomic.AddInt64(&self.size, 1)
}


type node struct {
	unsafe.Pointer
	value Thing
	deleted bool
}
func (self *node) next() *node {
	current := (*node)(atomic.LoadPointer(&self.Pointer))
	for current != nil && current.deleted {
		atomic.CompareAndSwapPointer(&self.Pointer, unsafe.Pointer(current), current.Pointer)		
		current = (*node)(atomic.LoadPointer(&self.Pointer))
	}
	return current
}
func (self *node) val() Thing {
	if self == nil {
		return nil
	}
	return self.value
}
func (self *node) String() string {
	return fmt.Sprint(self.ToSlice())
}
func (self *node) Describe() string {
	deleted := ""
	if self.deleted {
		deleted = " (x)"
	}
	return fmt.Sprintf("%#v -> %#v", self.value, deleted, self.next())
}
func (self *node) add(c Thing) {
	alloc := &node{}
	for !self.addBefore(c, alloc, self.next()) {}
}
func (self *node) addBefore(t Thing, allocatedNode, before *node) bool {
	if self.next() != before {
		return false
	}
	allocatedNode.value = t
	allocatedNode.Pointer = unsafe.Pointer(before)
	allocatedNode.deleted = false
	return atomic.CompareAndSwapPointer(&self.Pointer, unsafe.Pointer(before), unsafe.Pointer(allocatedNode))
}
/*
 * inject c into self either before the first matching value (c.Compare(value) == 0), before the first value
 * it should be before (c.Compare(value) < 0) or after the first value it should be after (c.Compare(value) > 0).
 */
func (self *node) inject(c Comparable) {
	alloc := &node{}
	for {
		hit := self.search(c)
		if hit.left != nil {
			if hit.node != nil {
				if hit.left.addBefore(c, alloc, hit.node) { break }
			} else {
				if hit.left.addBefore(c, alloc, hit.right) { break }
			}
		} else if hit.node != nil {
			if hit.node.addBefore(c, alloc, hit.right) { break }
		} else {
			panic(fmt.Errorf("Unable to inject %v properly into %v, it ought to be first but was injected into the first node of the list!", c, self))
		}
	}
}
func (self *node) ToSlice() []Thing {
	rval := make([]Thing, 0)
	current := self
	for current != nil {
		rval = append(rval, current.value)
		current = current.next()
	}
	return rval
}
/*
 * search for c in self.
 *
 * Will stop searching when finding nil or an element that should be after c (c.Compare(element) < 0).
 *
 * Will return a hit containing the last nodeRef and node before a match (if no match, the last nodeRef and node before
 * it stops searching), the nodeRef and node for the match (if a match) and the last nodeRef and node after the match
 * (if no match, the first nodeRef and node, or nil/nil if at the end of the list).
 */
func (self *node) search(c Comparable) (rval *hit) {
	rval = &hit{nil, self, nil}
	for {
		if rval.node == nil {
			return
		}
		rval.right = rval.node.next()
		switch cmp := c.Compare(rval.node.value); {
		case cmp < 0:
			rval.right = rval.node
			rval.node = nil
			return
		case cmp == 0:
			return
		}
		rval.left = rval.node
		rval.node = rval.left.next()
		rval.right = nil
	}
	panic(fmt.Sprint("Unable to search for ", c, " in ", self))
}
/*
 * Verify that all Comparable values in this list are after values they should be after (c.Compare(last) >= 0).
 */
func (self *node) verify() (err error) {
	current := self
	var last Thing
	var bad [][]Thing
	for current != nil {
		value := current.value
		if comp, ok := value.(Comparable); ok {
			if comp.Compare(last) < 0 {
				bad = append(bad, []Thing{last,value})
			}
		}
		last = value
		current = current.next()
	}
	if len(bad) == 0 {
		return nil
	}
	buffer := new(bytes.Buffer)
	for index, pair := range bad {
		fmt.Fprint(buffer, pair[0], ",", pair[1])
		if index < len(bad) - 1 {
			fmt.Fprint(buffer, "; ")
		}
	}
	return fmt.Errorf("%v is badly ordered. The following nodes are in the wrong order: %v", self, string(buffer.Bytes()));
	
}
func (self *node) removeExact(allocatedNode, oldNode *node) bool {
	if oldNode == nil {
		return false
	}
	allocatedNode.value = oldNode.value
	allocatedNode.Pointer = oldNode.Pointer
	allocatedNode.deleted = true
	return atomic.CompareAndSwapPointer(&self.Pointer, unsafe.Pointer(oldNode), unsafe.Pointer(allocatedNode))
}
func (self *node) remove() Thing {
	n := self.next()
	if n == nil {
		return nil
	}
	alloc := &node{}
	for !self.removeExact(alloc, n) {
		n = self.next()
	}
	if n != nil {
		return n.value
	}
	return nil
}
