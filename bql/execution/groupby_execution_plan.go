package execution

import (
	"fmt"
	"pfi/sensorbee/sensorbee/bql/parser"
	"pfi/sensorbee/sensorbee/bql/udf"
	"pfi/sensorbee/sensorbee/core"
	"pfi/sensorbee/sensorbee/data"
	"reflect"
	"time"
)

type groupbyExecutionPlan struct {
	commonExecutionPlan
	groupMode    bool
	emitterType  parser.Emitter
	emitterRules map[string]parser.IntervalAST
	emitCounters map[string]int64
	// store name->alias mapping
	relations []parser.AliasedStreamWindowAST
	// buffers holds data of a single stream window, keyed by the
	// alias (!) of the respective input stream. It will be
	// updated (appended and possibly truncated) whenever
	// Process() is called with a new tuple.
	buffers map[string]*inputBuffer
	// curResults holds results of a query over the buffer.
	curResults []data.Map
	// prevResults holds results of a query over the buffer
	// in the previous execution run.
	prevResults []data.Map
}

// tmpGroupData is an intermediate data structure to represent
// a set of rows that have the same values for GROUP BY columns.
type tmpGroupData struct {
	// this is the group (e.g. [1, "toy"]), where the values are
	// in order of the items in the GROUP BY clause
	group data.Array
	// for each aggregate function, we hold an array with the
	// input values.
	aggData map[string][]data.Value
	// as per our assumptions about grouping, the non-aggregation
	// data should be identical within every group
	nonAggData data.Map
}

// CanBuildGroupbyExecutionPlan checks whether the given statement
// allows to use an groupbyExecutionPlan.
func CanBuildGroupbyExecutionPlan(lp *LogicalPlan, reg udf.FunctionRegistry) bool {
	return lp.Having == nil
}

// groupbyExecutionPlan is a very simple plan that follows the
// theoretical processing model.
//
// After each tuple arrives,
// - compute the contents of the current window using the
//   specified window size/type,
// - perform a SELECT query on that data,
// - compute the data that need to be emitted by comparison with
//   the previous run's results.
func NewGroupbyExecutionPlan(lp *LogicalPlan, reg udf.FunctionRegistry) (ExecutionPlan, error) {
	// prepare projection components
	projs, err := prepareProjections(lp.Projections, reg)
	if err != nil {
		return nil, err
	}
	// compute evaluator for the filter
	filter, err := prepareFilter(lp.Filter, reg)
	if err != nil {
		return nil, err
	}
	// compute evaluators for the group clause
	groupList, err := prepareGroupList(lp.GroupList, reg)
	if err != nil {
		return nil, err
	}
	// for compatibility with the old syntax, take the last RANGE
	// specification as valid for all buffers

	// initialize buffers (one per declared input relation)
	buffers := make(map[string]*inputBuffer, len(lp.Relations))
	for _, rel := range lp.Relations {
		var tuples []*core.Tuple
		rangeValue := rel.Value
		rangeUnit := rel.Unit
		if rangeUnit == parser.Tuples {
			// we already know the required capacity of this buffer
			// if we work with absolute numbers
			tuples = make([]*core.Tuple, 0, rangeValue+1)
		}
		// the alias of the relation is the key of the buffer
		buffers[rel.Alias] = &inputBuffer{
			tuples, rangeValue, rangeUnit,
		}
	}
	emitterRules := make(map[string]parser.IntervalAST, len(lp.EmitIntervals))
	emitCounters := make(map[string]int64, len(lp.EmitIntervals))
	if len(lp.EmitIntervals) == 0 {
		// set the default if not `EVERY ...` was given
		emitterRules["*"] = parser.IntervalAST{parser.NumericLiteral{1}, parser.Tuples}
		emitCounters["*"] = 0
	}
	for _, emitRule := range lp.EmitIntervals {
		// TODO implement time-based emitter as well
		if emitRule.Unit == parser.Seconds {
			return nil, fmt.Errorf("time-based emitter not implemented")
		}
		emitterRules[emitRule.Name] = emitRule.IntervalAST
		emitCounters["*"] = 0
	}
	return &groupbyExecutionPlan{
		commonExecutionPlan: commonExecutionPlan{
			projections: projs,
			groupList:   groupList,
			filter:      filter,
		},
		groupMode:    lp.GroupingStmt,
		emitterType:  lp.EmitterType,
		emitterRules: emitterRules,
		emitCounters: emitCounters,
		relations:    lp.Relations,
		buffers:      buffers,
		curResults:   []data.Map{},
		prevResults:  []data.Map{},
	}, nil
}

// Process takes an input tuple and returns a slice of Map values that
// correspond to the results of the query represented by this execution
// plan. Note that the order of items in the returned slice is undefined
// and cannot be relied on.
func (ep *groupbyExecutionPlan) Process(input *core.Tuple) ([]data.Map, error) {
	// stream-to-relation:
	// updates the internal buffer with correct window data
	if err := ep.addTupleToBuffer(input); err != nil {
		return nil, err
	}
	if err := ep.removeOutdatedTuplesFromBuffer(input.Timestamp); err != nil {
		return nil, err
	}

	if ep.shouldEmitNow(input) {
		// relation-to-relation:
		// performs a SELECT query on buffer and writes result
		// to temporary table
		if err := ep.performQueryOnBuffer(); err != nil {
			return nil, err
		}

		// relation-to-stream:
		// compute new/old/all result data and return it
		// TODO use an iterator/generator pattern instead
		return ep.computeResultTuples()
	}
	return nil, nil
}

// relationKey computes the InputName that belongs to a relation.
// For a real stream this equals the stream's name (independent of)
// the alias, but for a UDSF we need to use the same method that
// was used in topologyBuilder.
func (ep *groupbyExecutionPlan) relationKey(rel *parser.AliasedStreamWindowAST) string {
	if rel.Type == parser.ActualStream {
		return rel.Name
	} else {
		return fmt.Sprintf("%s/%s", rel.Name, rel.Alias)
	}
}

// addTupleToBuffer appends the received tuple to all internal buffers that
// are associated to the tuple's input name (more than one on self-join).
// Note that after calling this function, these buffers may hold more
// items than allowed by the window specification, so a call to
// removeOutdatedTuplesFromBuffer is necessary afterwards.
func (ep *groupbyExecutionPlan) addTupleToBuffer(t *core.Tuple) error {
	// we need to append this tuple to all buffers where the input name
	// matches the relation name, so first we count the those buffers
	// (for `FROM a AS left, a AS right`, this tuple will be
	// appended to the two buffers for `left` and `right`)
	numAppends := 0
	for _, rel := range ep.relations {
		if t.InputName == ep.relationKey(&rel) {
			numAppends += 1
		}
	}
	// if the tuple's input name didn't match any known relation,
	// something is wrong in the topology and we should return an error
	if numAppends == 0 {
		knownRelNames := make([]string, 0, len(ep.relations))
		for _, rel := range ep.relations {
			knownRelNames = append(knownRelNames, rel.Name)
		}
		return fmt.Errorf("tuple has input name '%s' set, but we "+
			"can only deal with %v", t.InputName, knownRelNames)
	}
	for _, rel := range ep.relations {
		if t.InputName == ep.relationKey(&rel) {
			// if we have numAppends > 1 (meaning: this tuple is used in a
			// self-join) we should work with a copy, otherwise we can use
			// the original item
			editTuple := t
			if numAppends > 1 {
				editTuple = t.Copy()
			}
			// nest the data in a one-element map using the alias as the key
			editTuple.Data = data.Map{rel.Alias: editTuple.Data}
			// TODO maybe a slice is not the best implementation for a queue?
			bufferPtr := ep.buffers[rel.Alias]
			bufferPtr.tuples = append(bufferPtr.tuples, editTuple)
		}
	}

	return nil
}

// removeOutdatedTuplesFromBuffer removes tuples from the buffer that
// lie outside the current window as per the statement's window
// specification.
func (ep *groupbyExecutionPlan) removeOutdatedTuplesFromBuffer(curTupTime time.Time) error {
	for _, buffer := range ep.buffers {
		curBufSize := int64(len(buffer.tuples))
		if buffer.windowType == parser.Tuples { // tuple-based window
			if curBufSize > buffer.windowSize {
				// we just need to take the last `windowSize` items:
				// {a, b, c, d} => {b, c, d}
				buffer.tuples = buffer.tuples[curBufSize-buffer.windowSize : curBufSize]
			}

		} else if buffer.windowType == parser.Seconds { // time-based window
			// copy all "sufficiently new" tuples to new buffer
			// TODO avoid the reallocation here
			newBuf := make([]*core.Tuple, 0, curBufSize)
			for _, tup := range buffer.tuples {
				dur := curTupTime.Sub(tup.Timestamp)
				if dur.Seconds() <= float64(buffer.windowSize) {
					newBuf = append(newBuf, tup)
				}
			}
			buffer.tuples = newBuf
		} else {
			return fmt.Errorf("unknown window type: %+v", *buffer)
		}
	}

	return nil
}

// shouldEmitNow returns true if the input tuple should trigger
// computation of output values.
func (ep *groupbyExecutionPlan) shouldEmitNow(t *core.Tuple) bool {
	// first check if we have a stream-independent rule
	// (e.g., `RSTREAM` or `RSTREAM [EVERY 2 TUPLES]`)
	if interval, ok := ep.emitterRules["*"]; ok {
		counter := ep.emitCounters["*"]
		nextCounter := counter + 1
		if nextCounter%interval.Value == 0 {
			ep.emitCounters["*"] = 0
			return true
		}
		ep.emitCounters["*"] = nextCounter
		return false
	}
	// if there was no such rule, check if there is a
	// rule for the input stream the tuple came from
	if interval, ok := ep.emitterRules[t.InputName]; ok {
		counter := ep.emitCounters[t.InputName]
		nextCounter := counter + 1
		if nextCounter%interval.Value == 0 {
			ep.emitCounters[t.InputName] = 0
			return true
		}
		ep.emitCounters[t.InputName] = nextCounter
		return false
	}
	// there is no general rule and no rule for the input
	// stream of the tuple, so don't do anything
	return false
}

// performQueryOnBuffer executes a SELECT query on the data of the tuples
// currently stored in the buffer. The query results (which is a set of
// data.Value, not core.Tuple) is stored in ep.curResults. The data
// that was stored in ep.curResults before this method was called is
// moved to ep.prevResults. Note that the order of values in ep.curResults
// is undefined.
//
// In case of an error the contents of ep.curResults will still be
// the same as before the call (so that the next run performs as
// if no error had happened), but the contents of ep.curResults are
// undefined.
//
// Currently performQueryOnBuffer can only perform SELECT ... WHERE ...
// queries without aggregate functions, GROUP BY, or HAVING clauses.
func (ep *groupbyExecutionPlan) performQueryOnBuffer() error {
	// reuse the allocated memory
	output := ep.prevResults[0:0]
	// remember the previous results
	ep.prevResults = ep.curResults

	// we need to make a cross product of the data in all buffers,
	// combine it to get an input like
	//  {"streamA": {data}, "streamB": {data}, "streamC": {data}}
	// and then run filter/projections on each of this items

	dataHolder := data.Map{}

	// function to compute cartesian product and do something on each
	// resulting item
	var procCartProd func([]string, func(data.Map) error) error

	procCartProd = func(remainingKeys []string, processItem func(data.Map) error) error {
		if len(remainingKeys) > 0 {
			// not all buffers have been visited yet
			myKey := remainingKeys[0]
			myBuffer := ep.buffers[myKey].tuples
			rest := remainingKeys[1:]
			for _, t := range myBuffer {
				// add the data of this tuple to dataHolder and recurse
				dataHolder[myKey] = t.Data[myKey]
				setMetadata(dataHolder, myKey, t)
				if err := procCartProd(rest, processItem); err != nil {
					return err
				}
			}

		} else {
			// all tuples have been visited and we should now have the data
			// of one cartesian product item in dataHolder
			if err := processItem(dataHolder); err != nil {
				return err
			}
		}
		return nil
	}

	// groups holds one item for every combination of values that
	// appear in the GROUP BY clause
	groups := []tmpGroupData{}

	// findOrCreateGroup looks up the group that has the given
	// groupValues in the `groups` list. if there is no such
	// group, a new one is created and a copy of the given map
	// is used as a representative of this group's values.
	findOrCreateGroup := func(groupValues []data.Value, d data.Map) (*tmpGroupData, error) {
		eq := Equal(binOp{}).(*compBinOp).cmpOp
		groupValuesArr := data.Array(groupValues)
		// find the correct group
		groupIdx := -1
		for i, groupData := range groups {
			equals, err := eq(groupData.group, groupValuesArr)
			if err != nil {
				return nil, err
			}
			if equals {
				groupIdx = i
				break
			}
		}
		// if there is no such group, create one
		if groupIdx < 0 {
			newGroup := tmpGroupData{
				// the values that make up this group
				groupValues,
				// the input values of the aggregate functions
				map[string][]data.Value{},
				// a representative set of values for this group for later evaluation
				// TODO actually we don't need the whole map,
				//      just the parts common to the whole group
				d.Copy(),
			}
			// initialize the map with the aggregate function inputs
			for _, proj := range ep.projections {
				for key := range proj.aggrEvals {
					newGroup.aggData[key] = make([]data.Value, 0, 1)
				}
			}
			groups = append(groups, newGroup)
			groupIdx = len(groups) - 1
		}

		// return a pointer to the (found or created) group
		return &groups[groupIdx], nil
	}

	// function to evaluate filter on the input data and do the computations
	// that are required on each input tuple. (those computations differ
	// depending on whether we are in grouping mode or not.)
	evalItem := func(d data.Map) error {
		// evaluate filter condition and convert to bool
		if ep.filter != nil {
			filterResult, err := ep.filter.Eval(d)
			if err != nil {
				return err
			}
			filterResultBool, err := data.ToBool(filterResult)
			if err != nil {
				return err
			}
			// if it evaluated to false, do not further process this tuple
			// (ToBool also evalutes the NULL value to false, so we don't
			// need to treat this specially)
			if !filterResultBool {
				return nil
			}
		}

		// if we arrive here, the input tuple fulfills the filter criteria.
		// we now must act differently depending on whether we are in
		// grouping mode or not. in grouping mode, compute only the GROUP BY
		// expressions and the input expressions for aggregate functions now.
		// in non-grouping mode, already compute the final output.

		if ep.groupMode {
			// now compute the expressions in the GROUP BY to find the correct
			// group to append to
			itemGroupValues := make([]data.Value, len(ep.groupList))
			for i, eval := range ep.groupList {
				// ordinary "flat" expression
				value, err := eval.Eval(d)
				if err != nil {
					return err
				}
				itemGroupValues[i] = value
			}

			itemGroup, err := findOrCreateGroup(itemGroupValues, d)
			if err != nil {
				return err
			}

			// now compute all the input data for the aggregate functions,
			// e.g. for `SELECT count(a) + max(b/2)`, compute `a` and `b/2`
			for _, proj := range ep.projections {
				if proj.hasAggregate {
					// this column involves an aggregate function, but there
					// may be multiple ones
					for key, agg := range proj.aggrEvals {
						value, err := agg.aggrEval.Eval(d)
						if err != nil {
							return err
						}
						// now we need to store this value in the output map
						itemGroup.aggData[key] = append(itemGroup.aggData[key], value)
					}
				}
			}

		} else {
			// otherwise, compute all the projection expressions
			result := data.Map(make(map[string]data.Value, len(ep.projections)))
			for _, proj := range ep.projections {
				value, err := proj.evaluator.Eval(d)
				if err != nil {
					return err
				}
				if err := assignOutputValue(result, proj.alias, value); err != nil {
					return err
				}
			}
			output = append(output, result)
		}
		return nil
	}

	evalGroup := func(group *tmpGroupData) error {
		result := data.Map(make(map[string]data.Value, len(ep.projections)))
		for _, proj := range ep.projections {
			// compute aggregate values
			if proj.hasAggregate {
				// this column involves an aggregate function, but there
				// may be multiple ones
				for key, agg := range proj.aggrEvals {
					aggregateInputs := group.aggData[key]
					_ = agg.aggrFun
					// TODO use the real function, not poor man's "count",
					//      and also return an error on failure
					result := data.Int(len(aggregateInputs))
					group.nonAggData[key] = result
					delete(group.aggData, key)
				}
			}
			// now evaluate this projection on  the flattened data
			value, err := proj.evaluator.Eval(group.nonAggData)
			if err != nil {
				return err
			}
			if err := assignOutputValue(result, proj.alias, value); err != nil {
				return err
			}
		}
		output = append(output, result)
		return nil
	}

	rollback := func() {
		// NB. ep.prevResults currently points to an slice with
		//     results from the previous run. ep.curResults points
		//     to the same slice. output points to a different slice
		//     with a different underlying array.
		//     in the next run, output will be reusing the underlying
		//     storage of the current ep.prevResults to hold results.
		//     therefore when we leave this function we must make
		//     sure that ep.prevResults and ep.curResults have
		//     different underlying arrays or ISTREAM/DSTREAM will
		//     return wrong results.
		ep.prevResults = output
	}

	// Note: `ep.buffers` is a map, so iterating over its keys may yield
	// different results in every run of the program. We cannot expect
	// a consistent order in which evalItem is run on the items of the
	// cartesian product.
	allStreams := make([]string, 0, len(ep.buffers))
	for key := range ep.buffers {
		allStreams = append(allStreams, key)
	}
	if err := procCartProd(allStreams, evalItem); err != nil {
		rollback()
		return err
	}

	// if we arrive here, then in non-grouping mode, the final result
	// is already in the `output` list. otherwise the input for the
	// aggregation functions is still in the `group` list and we need
	// to compute aggregation and output now.
	if ep.groupMode {
		for _, group := range groups {
			if err := evalGroup(&group); err != nil {
				rollback()
				return err
			}
		}
	}

	ep.curResults = output
	return nil
}

// computeResultTuples compares the results of this run's query with
// the results of the previous run's query and returns the data to
// be emitted as per the Emitter specification (Rstream = new,
// Istream = new-old, Dstream = old-new).
//
// Currently there is no support for multiplicities, i.e., if an item
// is 3 times in `new` and 1 time in `old` it will *not* be contained
// in the result set.
func (ep *groupbyExecutionPlan) computeResultTuples() ([]data.Map, error) {
	// TODO turn this into an iterator/generator pattern
	var output []data.Map
	if ep.emitterType == parser.Rstream {
		// emit all tuples
		for _, res := range ep.curResults {
			output = append(output, res)
		}
	} else if ep.emitterType == parser.Istream {
		// emit only new tuples
		for _, res := range ep.curResults {
			// check if this tuple is already present in the previous results
			found := false
			for _, prevRes := range ep.prevResults {
				if reflect.DeepEqual(res, prevRes) {
					// yes, it is, do not emit
					// TODO we may want to delete the found item from prevRes
					//      so that item counts are considered for "new items"
					found = true
					break
				}
			}
			if found {
				continue
			}
			// if we arrive here, `res` is not contained in prevResults
			output = append(output, res)
		}
	} else if ep.emitterType == parser.Dstream {
		// emit only old tuples
		for _, prevRes := range ep.prevResults {
			// check if this tuple is present in the current results
			found := false
			for _, res := range ep.curResults {
				if reflect.DeepEqual(res, prevRes) {
					// yes, it is, do not emit
					// TODO we may want to delete the found item from curRes
					//      so that item counts are considered for "new items",
					//      but can we do this safely with regard to the next run?
					found = true
					break
				}
			}
			if found {
				continue
			}
			// if we arrive here, `prevRes` is not contained in curResults
			output = append(output, prevRes)
		}
	} else {
		return nil, fmt.Errorf("emitter type '%s' not implemented", ep.emitterType)
	}
	return output, nil
}
