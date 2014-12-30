package optimization

import (
	"fmt"
	"math"
	"sort"
)

const (
	alpha   = 1     // reflection, must be > 0
	beta    = 2     // expansion, must be > 1
	gamma   = .5    // contraction, 0 < gamma < 1
	sigma   = .5    // shrink, 0 < sigma < 1
	delta   = .0001 // going to use this to determine convergence
	maxRuns = 130
)

func min(vertices ...*nmVertex) *nmVertex {
	min := vertices[0]
	for _, v := range vertices[1:] {
		if v.distance < min.distance {
			min = v
		}
	}

	return min
}

// findMidpoint will find the midpoint of the provided vertices
// and return a new vertex.
func findMidpoint(vertices ...*nmVertex) *nmVertex {
	num := len(vertices) // this is what we divide by
	vars := make([]float64, 0, num)

	for i := 0; i < num; i++ {
		sum := float64(0)
		for _, v := range vertices {
			sum += v.vars[i]
		}
		vars = append(vars, sum/float64(num))
	}

	return &nmVertex{
		vars: vars,
	}
}

// determineDistance will determine the distance between the value
// and the target.  If the target is positive or negative infinity,
// (ie find max or min), this is clamped to max or min float64.
func determineDistance(value, target float64) float64 {
	if math.IsInf(target, 1) { // positive infinity
		target = math.MaxFloat64
	} else if math.IsInf(target, -1) { // negative infinity
		target = -math.MaxFloat64
	}

	return math.Abs(target - value)
}

type vertices []*nmVertex

// evaluate will call evaluate on all the verticies in this list
// and order them by distance to target.
func (vertices vertices) evaluate(config NelderMeadConfiguration) {
	for _, v := range vertices {
		v.evaluate(config)
	}

	vertices.sort()
}

func (vertices vertices) sort() {
	sort.Sort(vertices)
}

// the following methods are required for sort.Interface.  We
// use the standard libraries sort here as it uses an adaptive
// sort and we really don't expect there to be a ton of dimensions
// here so mulithreaded sort in this repo really isn't
// necessary.

func (vertices vertices) Less(i, j int) bool {
	return vertices[i].distance < vertices[j].distance
}

func (vertices vertices) Len() int {
	return len(vertices)
}

func (vertices vertices) Swap(i, j int) {
	vertices[i], vertices[j] = vertices[j], vertices[i]
}

func (vertices vertices) String() string {
	result := ``
	for i, v := range vertices {
		result += fmt.Sprintf(`VERTEX INDEX: %+v, VERTEX: %+v`, i, v)
		result += fmt.Sprintln(``)
	}

	return result
}

// NelderMeadConfiguration is the struct that must be
// passed into the NelderMead function.  This defines
// the target value, the function to be run, and a guess
// of the variables.
type NelderMeadConfiguration struct {
	// Target is the target we are trying to converge
	// to.  Set this to positive or negative infinity
	// to find the min/max.
	Target float64
	// Fn defines the function that Nelder Mead is going
	// to call to determine if it is moving closer
	// to convergence.  In all likelihood, the execution
	// of this function is going to be the bottleneck.
	Fn func([]float64) float64
	// Vars is a guess and will determine what other
	// vertices will be used.  By convention, since
	// this guess will contain as many numbers as the
	// target function requires, the len of Vars determines
	// the dimension of this problem.
	Vars []float64
}

type nmVertex struct {
	vars     []float64
	distance float64
	result   float64
}

func (nm *nmVertex) evaluate(config NelderMeadConfiguration) {
	nm.result = config.Fn(nm.vars)
	nm.distance = determineDistance(nm.result, config.Target)
}

func (nm *nmVertex) add(other *nmVertex) *nmVertex {
	vars := make([]float64, 0, len(nm.vars))
	for i := 0; i < len(nm.vars); i++ {
		vars = append(vars, nm.vars[i]+other.vars[i])
	}

	return &nmVertex{
		vars: vars,
	}
}

func (nm *nmVertex) multiply(scalar float64) *nmVertex {
	vars := make([]float64, 0, len(nm.vars))
	for i := 0; i < len(nm.vars); i++ {
		vars = append(vars, nm.vars[i]*scalar)
	}

	return &nmVertex{
		vars: vars,
	}
}

func (nm *nmVertex) subtract(other *nmVertex) *nmVertex {
	vars := make([]float64, 0, len(nm.vars))
	for i := 0; i < len(nm.vars); i++ {
		vars = append(vars, nm.vars[i]-other.vars[i])
	}

	return &nmVertex{
		vars: vars,
	}
}

type nelderMead struct {
	config   NelderMeadConfiguration
	vertices vertices
}

func (nm *nelderMead) reflect(midpoint *nmVertex) *nmVertex {
	toScalar := midpoint.subtract(nm.lastVertex())
	toScalar = toScalar.multiply(alpha)
	toScalar = midpoint.add(toScalar)
	toScalar.evaluate(nm.config)
	return toScalar
}

func (nm *nelderMead) expand(midpoint, reflection *nmVertex) *nmVertex {
	toScalar := reflection.subtract(midpoint)
	toScalar = toScalar.multiply(beta)
	toScalar = midpoint.add(toScalar)
	toScalar.evaluate(nm.config)
	return toScalar
}

// lastDimensionVertex returns the vertex that is represented by the
// last dimension, effectively, second to last in the list of
// vertices.
func (nm *nelderMead) lastDimensionVertex() *nmVertex {
	return nm.vertices[len(nm.vertices)-2]
}

// lastVertex returns the last vertex in the list of vertices.
// It's important to remember that this vertex represents the
// number of dimensions + 1.
func (nm *nelderMead) lastVertex() *nmVertex {
	return nm.vertices[len(nm.vertices)-1]
}

func (nm *nelderMead) outsideContract(midpoint, reflection *nmVertex) *nmVertex {
	toScalar := reflection.subtract(midpoint)
	toScalar = toScalar.multiply(gamma)
	toScalar = midpoint.add(toScalar)
	toScalar.evaluate(nm.config)
	return toScalar
}

func (nm *nelderMead) insideContract(midpoint, reflection *nmVertex) *nmVertex {
	toScalar := reflection.subtract(midpoint)
	toScalar = toScalar.multiply(gamma)
	toScalar = midpoint.subtract(toScalar)
	toScalar.evaluate(nm.config)
	return toScalar
}

func (nm *nelderMead) shrink() {
	one := nm.vertices[0]
	for i := 1; i < len(nm.vertices); i++ {
		toScalar := nm.vertices[i].subtract(one)
		toScalar = toScalar.multiply(sigma)
		nm.vertices[i] = one.add(toScalar)
	}
}

// checkIteration checks some key values to determine if
// iteration should be complete.  Returns false if iteration
// should be terminated and true if iteration should continue.
func (nm *nelderMead) checkIteration() bool {
	if math.Abs(nm.vertices[0].result-nm.config.Target) < delta {
		return false
	}

	best := nm.vertices[0]
	for _, v := range nm.vertices[1:] {
		if math.Abs(best.distance-v.distance) >= delta {
			return true
		}
	}

	return false
}

func (nm *nelderMead) evaluate() {
	for i := 0; i <= maxRuns; i++ {
		nm.vertices.evaluate(nm.config)
		if !nm.checkIteration() {
			break
		}

		midpoint := findMidpoint(nm.vertices[:len(nm.vertices)-1]...)
		// we are guaranteed to have two points here
		reflection := nm.reflect(midpoint)
		// in this case, quality-wise, we are between the best
		// and second to best points
		if reflection.distance < nm.lastDimensionVertex().distance &&
			reflection.distance >= nm.vertices[0].distance {

			nm.vertices[len(nm.vertices)-1] = reflection
			continue
		}

		// midpoint is closer than our previous best guess
		if reflection.distance < nm.vertices[0].distance {
			expanded := nm.expand(midpoint, reflection)

			// we only need to expand here
			if expanded.distance < reflection.distance {
				nm.vertices[len(nm.vertices)-1] = expanded
			} else {
				nm.vertices[len(nm.vertices)-1] = reflection
			}
			continue
		}

		// reflection is a bad guess, let's try to contract both
		// inside and outside and see if we can find a better value
		if reflection.distance < nm.lastVertex().distance {
			oc := nm.outsideContract(midpoint, reflection)
			if oc.distance <= reflection.distance {
				nm.vertices[len(nm.vertices)-1] = oc
				continue
			}
		} else if reflection.distance >= nm.lastVertex().distance {
			ic := nm.insideContract(midpoint, reflection)
			if ic.distance < nm.lastVertex().distance {
				nm.vertices[len(nm.vertices)-1] = ic
				continue
			}
		}

		// we could not guess a better value than nm.vertices[0], so
		// let's converge the other to guesses to our best guess.
		nm.shrink()
	}
}

func newNelderMead(config NelderMeadConfiguration) *nelderMead {
	vertices := make(vertices, 0, len(config.Vars)+1)
	v := &nmVertex{vars: config.Vars} // construct initial vertex with first guess
	vertices = append(vertices, v)
	for i := 0; i < len(config.Vars); i++ { // we ultimately have one more vertex than number of dimensions
		neg := i%2 == 0
		vars := make([]float64, 0, len(config.Vars))
		for i, v := range config.Vars {
			if i%2 == 0 && neg { // we must ensure all vertices do not fall on the same line
				vars = append(vars, -(v + float64(i) + 1))
			} else {
				vars = append(vars, v+float64(i)+1)
			}

		}
		vertices = append(vertices, &nmVertex{vars: vars})
	}

	return &nelderMead{
		config:   config,
		vertices: vertices,
	}
}

// NelderMead takes a configuration and returns a list
// of floats that can be plugged into the provided function
// to converge at the target value.
func NelderMead(config NelderMeadConfiguration) []float64 {
	nm := newNelderMead(config)
	nm.evaluate()
	return nm.vertices[0].vars
}
