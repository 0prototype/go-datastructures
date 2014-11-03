package graph

import "github.com/Workiva/go-datastructures/bitarray"

// IExecutionGraph represents a subgraph that can have some function
// applied across the nodes.
type IExecutionGraph interface {
	// ParallelRecursivelyApply will apply the provided function in a parallel
	// fashion to the nodes of this graph.  Return false to halt application.
	ParallelRecursivelyApply(fn func(node INode) bool)
	// RecursivelyApply will apply the supplied function in linear fashion.
	// Return false to halt application.
	RecursivelyApply(fn func(node INode) bool)
	// Size returns the number of nodes in this graph.
	Size() int64
}

// INode represents an object that can live within the graph.
type INode interface {
	// SetCircular sets a value indicating if this node is recursive
	SetCircular(value bool)
	// IsCircular returns a value indicating if this node is recursive
	IsCircular() bool
	// ID returns a strictly monotonically increasing integer that is unique to every node.
	ID() uint64
}

// IDependencyProvider aids the graph code in setting and tracking dependencies
type IDependencyProvider interface {
	// MaxNode returns the highest node ID possible.
	MaxNode() uint64
	// GetDependencies will return a bit array representing the dependencies of the provided node.
	GetDependencies(node INode) []bitarray.BitArray
	// GetDependents will return a list of all nodes that depend in some
	// way on the provided nodes.
	GetDependents(nodes Nodes) Nodes
}
