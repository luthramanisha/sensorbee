package execution

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"pfi/sensorbee/sensorbee/bql/parser"
	"pfi/sensorbee/sensorbee/bql/udf"
	"pfi/sensorbee/sensorbee/data"
	"sort"
	"testing"
	"time"
)

func createGroupbyPlan(s string, t *testing.T) (ExecutionPlan, error) {
	p := parser.NewBQLParser()
	reg := udf.CopyGlobalUDFRegistry(newTestContext())
	_stmt, _, err := p.ParseStmt(s)
	So(err, ShouldBeNil)
	So(_stmt, ShouldHaveSameTypeAs, parser.CreateStreamAsSelectStmt{})
	stmt := _stmt.(parser.CreateStreamAsSelectStmt)
	logicalPlan, err := Analyze(stmt)
	So(err, ShouldBeNil)
	canBuild := CanBuildGroupbyExecutionPlan(logicalPlan, reg)
	So(canBuild, ShouldBeTrue)
	return NewGroupbyExecutionPlan(logicalPlan, reg)
}

func TestGroupbyExecutionPlan(t *testing.T) {
	// Select constant
	Convey("Given a SELECT clause with a constant", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM 2, null FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then that constant should appear in %v", idx), func() {
					if idx == 0 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"col_1": data.Int(2), "col_2": data.Null{}})
					} else {
						// nothing should be emitted because no new
						// data appears
						So(len(out), ShouldEqual, 0)
					}
				})
			}

		})
	})

	// Select a column with changing values
	Convey("Given a SELECT clause with only a column", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM int FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"int": data.Int(idx + 1)})
				})
			}

		})
	})

	Convey("Given a SELECT clause with only a column and GROUP BY", t, func() {
		tuples := getTuples(4)
		tuples[0].Data["foo"] = data.Int(1)
		tuples[1].Data["foo"] = data.Int(1)
		tuples[2].Data["foo"] = data.Int(2)
		tuples[3].Data["foo"] = data.Int(2)
		s := `CREATE STREAM box AS SELECT RSTREAM foo, count(int + 1) + 2 FROM src [RANGE 3 TUPLES] GROUP BY foo`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					if idx == 0 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"foo": data.Int(1), "col_2": data.Int(3)})
					} else if idx == 1 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"foo": data.Int(1), "col_2": data.Int(4)})
					} else if idx == 2 {
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble,
							data.Map{"foo": data.Int(1), "col_2": data.Int(4)})
						So(out[1], ShouldResemble,
							data.Map{"foo": data.Int(2), "col_2": data.Int(3)})
					} else {
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble,
							data.Map{"foo": data.Int(1), "col_2": data.Int(3)})
						So(out[1], ShouldResemble,
							data.Map{"foo": data.Int(2), "col_2": data.Int(4)})
					}
				})
			}
		})
	})

	Convey("Given a SELECT clause with only a column and GROUP BY (2 cols)", t, func() {
		tuples := getTuples(4)
		tuples[0].Data["foo"] = data.Int(1)
		tuples[0].Data["bar"] = data.Int(1)
		tuples[1].Data["foo"] = data.Int(1)
		tuples[1].Data["bar"] = data.Int(1)
		tuples[2].Data["foo"] = data.Int(2)
		tuples[2].Data["bar"] = data.Int(1)
		tuples[3].Data["foo"] = data.Int(2)
		tuples[3].Data["bar"] = data.Int(2)
		s := `CREATE STREAM box AS SELECT RSTREAM foo, count(int) + 2 AS x FROM src [RANGE 3 TUPLES] GROUP BY foo, bar`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					if idx == 0 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"foo": data.Int(1), "x": data.Int(3)})
					} else if idx == 1 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"foo": data.Int(1), "x": data.Int(4)})
					} else if idx == 2 {
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble,
							data.Map{"foo": data.Int(1), "x": data.Int(4)})
						So(out[1], ShouldResemble,
							data.Map{"foo": data.Int(2), "x": data.Int(3)})
					} else {
						So(len(out), ShouldEqual, 3)
						So(out[0], ShouldResemble,
							data.Map{"foo": data.Int(1), "x": data.Int(3)})
						So(out[1], ShouldResemble,
							data.Map{"foo": data.Int(2), "x": data.Int(3)})
						So(out[2], ShouldResemble,
							data.Map{"foo": data.Int(2), "x": data.Int(3)})
					}
				})
			}
		})
	})

	Convey("Given a SELECT clause with only a column and GROUP BY (backref)", t, func() {
		tuples := getTuples(4)
		tuples[0].Data["foo"] = data.Int(1)
		tuples[1].Data["foo"] = data.Int(1)
		tuples[2].Data["foo"] = data.Int(2)
		tuples[3].Data["foo"] = data.Int(2)
		s := `CREATE STREAM box AS SELECT RSTREAM foo AS y, count(int + 1) + foo FROM src [RANGE 3 TUPLES] GROUP BY foo`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					if idx == 0 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"y": data.Int(1), "col_2": data.Int(2)})
					} else if idx == 1 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"y": data.Int(1), "col_2": data.Int(3)})
					} else if idx == 2 {
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble,
							data.Map{"y": data.Int(1), "col_2": data.Int(3)})
						So(out[1], ShouldResemble,
							data.Map{"y": data.Int(2), "col_2": data.Int(3)})
					} else {
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble,
							data.Map{"y": data.Int(1), "col_2": data.Int(2)})
						So(out[1], ShouldResemble,
							data.Map{"y": data.Int(2), "col_2": data.Int(4)})
					}
				})
			}
		})
	})

	Convey("Given a SELECT clause with only a column using the table name", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM src:int FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"int": data.Int(idx + 1)})
				})
			}

		})
	})

	// Select the tuple's timestamp
	Convey("Given a SELECT clause with only the timestamp", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM ts() FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"ts": data.Timestamp(time.Date(2015, time.April, 10,
							10, 23, idx, 0, time.UTC))})
				})
			}

		})
	})

	// Select a non-existing column
	Convey("Given a SELECT clause with a non-existing column", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM hoge FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for _, inTup := range tuples {
				_, err := plan.Process(inTup)
				So(err, ShouldNotBeNil) // hoge not found
			}

		})
	})

	Convey("Given a SELECT clause with a non-existing column", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM hoge + 1 FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for _, inTup := range tuples {
				_, err := plan.Process(inTup)
				So(err, ShouldNotBeNil) // hoge not found
			}

		})
	})

	// Select constant and a column with changing values
	Convey("Given a SELECT clause with a constant and a column", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM 2, int FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"col_1": data.Int(2), "int": data.Int(idx + 1)})
				})
			}

		})
	})

	// Select constant and a column with changing values from aliased relation
	Convey("Given a SELECT clause with a constant, a column, and a table alias", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM 2, int FROM src [RANGE 2 SECONDS] AS x`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"col_1": data.Int(2), "int": data.Int(idx + 1)})
				})
			}

		})
	})

	// Select NULL-related operations
	Convey("Given a SELECT clause with NULL operations", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM null IS NULL, null + 2 = 2 FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then the null operations should be correct %v", idx), func() {
					if idx == 0 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"col_1": data.Bool(true), "col_2": data.Null{}})
					} else {
						// nothing should be emitted because no new
						// data appears
						So(len(out), ShouldEqual, 0)
					}
				})
			}

		})
	})

	Convey("Given a SELECT clause with NULL filter", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM int FROM src [RANGE 2 SECONDS] WHERE null`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then there should be no rows in the output %v", idx), func() {
					So(len(out), ShouldEqual, 0)
				})
			}

		})
	})

	// Select constant and a column with changing values from aliased relation
	// using that alias
	Convey("Given a SELECT clause with a constant, a table alias, and a column using it", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM 2, x:int FROM src [RANGE 2 SECONDS] AS x`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"col_1": data.Int(2), "int": data.Int(idx + 1)})
				})
			}

		})
	})

	// Use alias
	Convey("Given a SELECT clause with a column alias", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM int-1 AS a, int AS b FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"a": data.Int(idx), "b": data.Int(idx + 1)})
				})
			}

		})
	})

	Convey("Given a SELECT clause with a nested column alias", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM int-1 AS a.c, int+1 AS a["d"], int AS b[1] FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"a": data.Map{"c": data.Int(idx), "d": data.Int(idx + 2)},
							"b": data.Array{data.Null{}, data.Int(idx + 1)}})
				})
			}

		})
	})

	// Use wildcard
	Convey("Given a SELECT clause with a wildcard", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM * FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"int": data.Int(idx + 1)})
				})
			}

		})
	})

	Convey("Given a SELECT clause with a wildcard and an overriding column", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM *, (int-1)*2 AS int FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"int": data.Int(2 * idx)})
				})
			}

		})
	})

	Convey("Given a SELECT clause with a column and an overriding wildcard", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM (int-1)*2 AS int, * FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"int": data.Int(idx + 1)})
				})
			}

		})
	})

	Convey("Given a SELECT clause with an aliased wildcard and an anonymous column", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM * AS x, (int-1)*2 FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					So(len(out), ShouldEqual, 1)
					So(out[0], ShouldResemble,
						data.Map{"col_2": data.Int(2 * idx), "x": data.Map{"int": data.Int(idx + 1)}})
				})
			}

		})
	})

	// Use a filter
	Convey("Given a SELECT clause with a column alias", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM int AS b FROM src [RANGE 2 SECONDS] 
            WHERE int % 2 = 0`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
					if (idx+1)%2 == 0 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"b": data.Int(idx + 1)})
					} else {
						So(len(out), ShouldEqual, 0)
					}
				})
			}

		})
	})
}

func TestGroupbyExecutionPlanEmitters(t *testing.T) {
	// Recovery from errors in tuples
	Convey("Given a SELECT clause with a column that does not exist in one tuple (RSTREAM)", t, func() {
		tuples := getTuples(6)
		// remove the selected key from one tuple
		delete(tuples[1].Data, "int")

		s := `CREATE STREAM box AS SELECT RSTREAM int FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)

				if idx == 0 {
					// In the idx==0 run, the window contains only item 0.
					// That item is fine, no problem.
					Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
						So(err, ShouldBeNil)
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"int": data.Int(idx + 1)})
					})
				} else if idx == 1 || idx == 2 {
					// In the idx==1 run, the window contains item 0 and item 1,
					// the latter is broken, therefore the query fails.
					// In the idx==2 run, the window contains item 1 and item 2,
					// the latter is broken, therefore the query fails.
					Convey(fmt.Sprintf("Then there should be an error for a queries in %v", idx), func() {
						So(err, ShouldNotBeNil)
					})
				} else {
					// In later runs, we have recovered from the error in item 1
					// and emit one item per run as normal.
					Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
						So(err, ShouldBeNil)
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble,
							data.Map{"int": data.Int(idx)})
						So(out[1], ShouldResemble,
							data.Map{"int": data.Int(idx + 1)})
					})
				}
			}

		})
	})

	Convey("Given a SELECT clause with a column that does not exist in one tuple (ISTREAM)", t, func() {
		tuples := getTuples(6)
		// remove the selected key from one tuple
		delete(tuples[1].Data, "int")

		s := `CREATE STREAM box AS SELECT ISTREAM int FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)

				if idx == 0 {
					// In the idx==0 run, the window contains only item 0.
					// That item is fine, no problem.
					Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
						So(err, ShouldBeNil)
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"int": data.Int(idx + 1)})
					})
				} else if idx == 1 || idx == 2 {
					// In the idx==1 run, the window contains item 0 and item 1,
					// the latter is broken, therefore the query fails.
					// In the idx==2 run, the window contains item 1 and item 2,
					// the latter is broken, therefore the query fails.
					Convey(fmt.Sprintf("Then there should be an error for a queries in %v", idx), func() {
						So(err, ShouldNotBeNil)
					})
				} else if idx == 3 {
					// In the idx==3 run, the window contains item 2 and item 3.
					// Both items are fine and have not been emitted before, so
					// both are emitted now.
					Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
						So(err, ShouldBeNil)
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble,
							data.Map{"int": data.Int(idx)})
						So(out[1], ShouldResemble,
							data.Map{"int": data.Int(idx + 1)})
					})
				} else {
					// In later runs, we have recovered from the error in item 1
					// and emit one item per run as normal.
					Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
						So(err, ShouldBeNil)
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"int": data.Int(idx + 1)})
					})
				}
			}

		})
	})

	Convey("Given a SELECT clause with a column that does not exist in one tuple (DSTREAM)", t, func() {
		tuples := getTuples(6)
		// remove the selected key from one tuple
		delete(tuples[1].Data, "int")

		s := `CREATE STREAM box AS SELECT DSTREAM int FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)

				if idx == 0 {
					// In the idx==0 run, the window contains only item 0.
					Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
						So(err, ShouldBeNil)
						So(len(out), ShouldEqual, 0)
					})
				} else if idx == 1 || idx == 2 {
					// In the idx==1 run, the window contains item 0 and item 1,
					// the latter is broken, therefore the query fails.
					// In the idx==2 run, the window contains item 1 and item 2,
					// the latter is broken, therefore the query fails.
					Convey(fmt.Sprintf("Then there should be an error for a queries in %v", idx), func() {
						So(err, ShouldNotBeNil)
					})
				} else if idx == 3 {
					// In the idx==3 run, the window contains item 2 and item 3.
					// Both items are fine and so item 0 is emitted.
					Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
						So(err, ShouldBeNil)
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"int": data.Int(1)})
					})
				} else {
					// In later runs, we have recovered from the error in item 1
					// and emit one item per run as normal.
					Convey(fmt.Sprintf("Then those values should appear in %v", idx), func() {
						So(err, ShouldBeNil)
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble,
							data.Map{"int": data.Int(idx - 1)})
					})
				}
			}

		})
	})

	// RSTREAM/2 SECONDS window
	Convey("Given an RSTREAM emitter selecting a constant and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT RSTREAM 2 AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 1)
				So(output[0][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 3)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[2][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[2][2], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[3]), ShouldEqual, 3)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[3][2], ShouldResemble, data.Map{"a": data.Int(2)})
			})

		})
	})

	Convey("Given an RSTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT RSTREAM int AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 1)
				So(output[0][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 3)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[2][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[2][2], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[3]), ShouldEqual, 3)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(3)})
				So(output[3][2], ShouldResemble, data.Map{"a": data.Int(4)})
			})

		})
	})

	// RSTREAM/2 TUPLES window
	Convey("Given an RSTREAM emitter selecting a constant and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT RSTREAM 2 AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 1)
				So(output[0][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 2)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[2][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[3]), ShouldEqual, 2)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(2)})
			})

		})
	})

	Convey("Given an RSTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT RSTREAM int AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole window state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 1)
				So(output[0][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 2)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[2][1], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[3]), ShouldEqual, 2)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(3)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(4)})
			})

		})
	})

	// ISTREAM/2 SECONDS window
	Convey("Given an ISTREAM emitter selecting a constant and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM 2 AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 1)
				So(output[0][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 0)
				So(len(output[3]), ShouldEqual, 0)
			})

		})
	})

	Convey("Given an ISTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM int AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 1)
				So(output[0][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(len(output[1]), ShouldEqual, 1)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 1)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[3]), ShouldEqual, 1)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(4)})
			})

		})
	})

	// ISTREAM/2 TUPLES window
	Convey("Given an ISTREAM emitter selecting a constant and a 2 TUPLES window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM 2 AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 1)
				So(output[0][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 0)
				So(len(output[3]), ShouldEqual, 0)
			})

		})
	})

	Convey("Given an ISTREAM emitter selecting a column and a 2 TUPLES window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT ISTREAM int AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 1)
				So(output[0][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(len(output[1]), ShouldEqual, 1)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 1)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[3]), ShouldEqual, 1)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(4)})
			})

		})
	})

	// DSTREAM/2 SECONDS window
	Convey("Given a DSTREAM emitter selecting a constant and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT DSTREAM 2 AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 0)
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 0)
				So(len(output[3]), ShouldEqual, 0)
			})

		})
	})

	Convey("Given a DSTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT DSTREAM int AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 0)
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 0)
				So(len(output[3]), ShouldEqual, 1)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(1)})
			})

		})
	})

	// DSTREAM/2 TUPLES window
	Convey("Given a DSTREAM emitter selecting a constant and a 2 TUPLES window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT DSTREAM 2 AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 0)
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 0)
				So(len(output[3]), ShouldEqual, 0)
			})

		})
	})

	Convey("Given a DSTREAM emitter selecting a column and a 2 TUPLES window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT DSTREAM int AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 0)
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 1)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(len(output[3]), ShouldEqual, 1)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(2)})
			})

		})
	})

	// Varying emitter intervals

	// RSTREAM [EVERY k TUPLES]/2 SECONDS window
	Convey("Given an RSTREAM emitter selecting a constant and a 2 SECONDS window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT RSTREAM [EVERY 2 TUPLES] 2 AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 3)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[3][2], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 3)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[5][2], ShouldResemble, data.Map{"a": data.Int(2)})
			})

		})
	})

	Convey("Given an RSTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT RSTREAM [EVERY 2 TUPLES] int AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 3)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(3)})
				So(output[3][2], ShouldResemble, data.Map{"a": data.Int(4)})
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 3)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(4)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(5)})
				So(output[5][2], ShouldResemble, data.Map{"a": data.Int(6)})
			})
		})
	})

	Convey("Given an RSTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT RSTREAM [EVERY 3 TUPLES] int AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 3)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[2][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[2][2], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 3)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(4)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(5)})
				So(output[5][2], ShouldResemble, data.Map{"a": data.Int(6)})
			})
		})
	})

	// RSTREAM [EVERY k TUPLES]/2 TUPLES window
	Convey("Given an RSTREAM emitter selecting a constant and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT RSTREAM [EVERY 2 TUPLES] 2 AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 2)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(2)})
			})

		})
	})

	Convey("Given an RSTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(4)
		s := `CREATE STREAM box AS SELECT RSTREAM [EVERY 2 TUPLES] int AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole window state should be emitted", func() {
				So(len(output), ShouldEqual, 4)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 2)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(3)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(4)})
			})

		})
	})

	Convey("Given an RSTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT RSTREAM [EVERY 3 TUPLES] int AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then the whole window state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 2)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[2][1], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 2)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(5)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(6)})
			})

		})
	})

	// ISTREAM [EVERY k TUPLES]/2 SECONDS window
	Convey("Given an ISTREAM emitter selecting a constant and a 2 SECONDS window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT ISTREAM [EVERY 2 TUPLES] 2 AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 0)
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 0)
			})

		})
	})

	Convey("Given an ISTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT ISTREAM [EVERY 2 TUPLES] int AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 2)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(3)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(4)})
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 2)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(5)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(6)})
			})

		})
	})

	Convey("Given an ISTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT ISTREAM [EVERY 3 TUPLES] int AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 3)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[2][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[2][2], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 3)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(4)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(5)})
				So(output[5][2], ShouldResemble, data.Map{"a": data.Int(6)})
			})

		})
	})

	// ISTREAM [EVERY k TUPLES]/2 TUPLES window
	Convey("Given an ISTREAM emitter selecting a constant and a 2 TUPLES window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT ISTREAM [EVERY 2 TUPLES] 2 AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 0)
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 0)
			})

		})
	})

	Convey("Given an ISTREAM emitter selecting a column and a 2 TUPLES window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT ISTREAM [EVERY 2 TUPLES] int AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 2)
				So(output[1][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[1][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 2)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(3)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(4)})
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 2)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(5)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(6)})
			})
		})
	})

	Convey("Given an ISTREAM emitter selecting a column and a 2 TUPLES window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT ISTREAM [EVERY 3 TUPLES] int AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then new items in state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 2)
				So(output[2][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[2][1], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 2)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(5)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(6)})
			})

		})
	})

	// DSTREAM [EVERY k TUPLES]/2 SECONDS window
	Convey("Given a DSTREAM emitter selecting a constant and a 2 SECONDS window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT DSTREAM [EVERY 2 TUPLES] 2 AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 0)
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 0)
			})

		})
	})

	Convey("Given a DSTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(6)
		s := `CREATE STREAM box AS SELECT DSTREAM [EVERY 2 TUPLES] int AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 6)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 1)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 2)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(3)})
			})

		})
	})

	Convey("Given a DSTREAM emitter selecting a column and a 2 SECONDS window", t, func() {
		tuples := getTuples(8)
		s := `CREATE STREAM box AS SELECT DSTREAM [EVERY 3 TUPLES] int AS a FROM src [RANGE 2 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 8)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 0)
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 3)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[5][2], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[6]), ShouldEqual, 0) // skip
				So(len(output[7]), ShouldEqual, 0) // skip
			})

		})
	})

	// DSTREAM [EVERY k TUPLES]/2 TUPLES window
	Convey("Given a DSTREAM emitter selecting a constant and a 2 TUPLES window", t, func() {
		tuples := getTuples(8)
		s := `CREATE STREAM box AS SELECT DSTREAM [EVERY 2 TUPLES] 2 AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 8)
				So(len(output[0]), ShouldEqual, 0)
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 0)
				So(len(output[3]), ShouldEqual, 0)
				So(len(output[4]), ShouldEqual, 0)
				So(len(output[5]), ShouldEqual, 0)
				So(len(output[6]), ShouldEqual, 0)
				So(len(output[7]), ShouldEqual, 0)
			})

		})
	})

	Convey("Given a DSTREAM emitter selecting a column and a 2 TUPLES window", t, func() {
		tuples := getTuples(8)
		s := `CREATE STREAM box AS SELECT DSTREAM [EVERY 2 TUPLES] int AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 8)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0)
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 2)
				So(output[3][0], ShouldResemble, data.Map{"a": data.Int(1)})
				So(output[3][1], ShouldResemble, data.Map{"a": data.Int(2)})
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 2)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(3)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(4)})
				So(len(output[6]), ShouldEqual, 0) // skip
				So(len(output[7]), ShouldEqual, 2)
				So(output[7][0], ShouldResemble, data.Map{"a": data.Int(5)})
				So(output[7][1], ShouldResemble, data.Map{"a": data.Int(6)})
			})

		})
	})

	Convey("Given a DSTREAM emitter selecting a column and a 2 TUPLES window", t, func() {
		tuples := getTuples(8)
		s := `CREATE STREAM box AS SELECT DSTREAM [EVERY 3 TUPLES] int AS a FROM src [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				output = append(output, out)
			}

			Convey("Then items dropped from state should be emitted", func() {
				So(len(output), ShouldEqual, 8)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 0)
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 2)
				So(output[5][0], ShouldResemble, data.Map{"a": data.Int(2)})
				So(output[5][1], ShouldResemble, data.Map{"a": data.Int(3)})
				So(len(output[6]), ShouldEqual, 0) // skip
				So(len(output[7]), ShouldEqual, 0) // skip
			})
		})
	})
}

func TestGroupbyExecutionPlanJoin(t *testing.T) {
	Convey("Given a JOIN selecting from left and right", t, func() {
		tuples := getTuples(8)
		// rearrange the tuples
		for i, t := range tuples {
			if i%2 == 0 {
				t.InputName = "src1"
				t.Data["l"] = data.String(fmt.Sprintf("l%d", i))
			} else {
				t.InputName = "src2"
				t.Data["r"] = data.String(fmt.Sprintf("r%d", i))
			}
		}
		s := `CREATE STREAM box AS SELECT ISTREAM src1:l, src2:r FROM src1 [RANGE 2 TUPLES], src2 [RANGE 2 TUPLES]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				// sort the output by the "l" and then the "r" key before
				// checking if it resembles the expected value
				sort.Sort(tupleList(out))
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then joined values should appear in %v", idx), func() {
					if idx == 0 {
						So(len(out), ShouldEqual, 0)
					} else if idx == 1 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l0"),
							"r": data.String("r1"),
						})
					} else if idx == 2 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l2"),
							"r": data.String("r1"),
						})
					} else if idx%2 == 1 {
						// a tuple from src2 (=right) was just added
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String(fmt.Sprintf("l%d", idx-3)),
							"r": data.String(fmt.Sprintf("r%d", idx)),
						})
						So(out[1], ShouldResemble, data.Map{
							"l": data.String(fmt.Sprintf("l%d", idx-1)),
							"r": data.String(fmt.Sprintf("r%d", idx)),
						})
					} else {
						// a tuple from src1 (=left) was just added
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String(fmt.Sprintf("l%d", idx)),
							"r": data.String(fmt.Sprintf("r%d", idx-3)),
						})
						So(out[1], ShouldResemble, data.Map{
							"l": data.String(fmt.Sprintf("l%d", idx)),
							"r": data.String(fmt.Sprintf("r%d", idx-1)),
						})
					}
				})
			}
		})
	})

	Convey("Given a JOIN selecting from left and right with different ranges", t, func() {
		tuples := getTuples(8)
		// rearrange the tuples
		for i, t := range tuples {
			if i%2 == 0 {
				t.InputName = "src1"
				t.Data["l"] = data.String(fmt.Sprintf("l%d", i))
			} else {
				t.InputName = "src2"
				t.Data["r"] = data.String(fmt.Sprintf("r%d", i))
			}
		}
		s := `CREATE STREAM box AS SELECT RSTREAM src1:l, src2:r FROM src1 [RANGE 1 TUPLES], src2 [RANGE 5 SECONDS]`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				// sort the output by the "l" and then the "r" key before
				// checking if it resembles the expected value
				sort.Sort(tupleList(out))
				So(err, ShouldBeNil)

				Convey(fmt.Sprintf("Then joined values should appear in %v", idx), func() {
					if idx == 0 { // l0
						So(len(out), ShouldEqual, 0)
					} else if idx == 1 { // r1
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l0"),
							"r": data.String("r1"),
						})
					} else if idx == 2 { // l2
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l2"),
							"r": data.String("r1"),
						})
					} else if idx == 3 { // r3
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l2"),
							"r": data.String("r1"),
						})
						So(out[1], ShouldResemble, data.Map{
							"l": data.String("l2"),
							"r": data.String("r3"),
						})
					} else if idx == 4 { // l4
						So(len(out), ShouldEqual, 2)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l4"),
							"r": data.String("r1"),
						})
						So(out[1], ShouldResemble, data.Map{
							"l": data.String("l4"),
							"r": data.String("r3"),
						})
					} else if idx == 5 { // r5
						So(len(out), ShouldEqual, 3)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l4"),
							"r": data.String("r1"),
						})
						So(out[1], ShouldResemble, data.Map{
							"l": data.String("l4"),
							"r": data.String("r3"),
						})
						So(out[2], ShouldResemble, data.Map{
							"l": data.String("l4"),
							"r": data.String("r5"),
						})
					} else if idx == 6 { // l6
						So(len(out), ShouldEqual, 3)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l6"),
							"r": data.String("r1"),
						})
						So(out[1], ShouldResemble, data.Map{
							"l": data.String("l6"),
							"r": data.String("r3"),
						})
						So(out[2], ShouldResemble, data.Map{
							"l": data.String("l6"),
							"r": data.String("r5"),
						})
					} else if idx == 7 { // r7
						So(len(out), ShouldEqual, 3)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l6"),
							"r": data.String("r3"),
						})
						So(out[1], ShouldResemble, data.Map{
							"l": data.String("l6"),
							"r": data.String("r5"),
						})
						So(out[2], ShouldResemble, data.Map{
							"l": data.String("l6"),
							"r": data.String("r7"),
						})
					}
				})
			}
		})
	})

	Convey("Given a JOIN selecting from left and right with different RSTREAM emitters and ranges", t, func() {
		tuples := getTuples(12)
		// rearrange the tuples
		for i, t := range tuples {
			if i%2 == 0 {
				t.InputName = "src1"
				t.Data["a"] = data.Int(i/2 + 1)
			} else {
				t.InputName = "src2"
				t.Data["b"] = data.Int(i/2 + 1)
			}
		}
		s := `CREATE STREAM box AS SELECT
		RSTREAM [EVERY 2 TUPLES IN src1, 3 TUPLES IN src2]
			x:a AS l, y:b AS r
		FROM src1 [RANGE 3 TUPLES] AS x, src2 [RANGE 2 TUPLES] AS y`

		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				// sort the output by the "l" and then the "r" key before
				// checking if it resembles the expected value
				sort.Sort(tupleList(out))
				output = append(output, out)
			}

			Convey("Then joined values should appear", func() {
				So(len(output), ShouldEqual, 12)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 2)
				So(output[2], ShouldResemble, []data.Map{
					{"l": data.Int(1), "r": data.Int(1)},
					{"l": data.Int(2), "r": data.Int(1)},
				})
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 6)
				So(output[5], ShouldResemble, []data.Map{
					{"l": data.Int(1), "r": data.Int(2)},
					{"l": data.Int(1), "r": data.Int(3)},
					{"l": data.Int(2), "r": data.Int(2)},
					{"l": data.Int(2), "r": data.Int(3)},
					{"l": data.Int(3), "r": data.Int(2)},
					{"l": data.Int(3), "r": data.Int(3)},
				})
				So(len(output[6]), ShouldEqual, 6)
				So(output[6], ShouldResemble, []data.Map{
					{"l": data.Int(2), "r": data.Int(2)},
					{"l": data.Int(2), "r": data.Int(3)},
					{"l": data.Int(3), "r": data.Int(2)},
					{"l": data.Int(3), "r": data.Int(3)},
					{"l": data.Int(4), "r": data.Int(2)},
					{"l": data.Int(4), "r": data.Int(3)},
				})
				So(len(output[7]), ShouldEqual, 0) // skip
				So(len(output[8]), ShouldEqual, 0) // skip
				So(len(output[9]), ShouldEqual, 0) // skip
				So(len(output[10]), ShouldEqual, 6)
				So(output[10], ShouldResemble, []data.Map{
					{"l": data.Int(4), "r": data.Int(4)},
					{"l": data.Int(4), "r": data.Int(5)},
					{"l": data.Int(5), "r": data.Int(4)},
					{"l": data.Int(5), "r": data.Int(5)},
					{"l": data.Int(6), "r": data.Int(4)},
					{"l": data.Int(6), "r": data.Int(5)},
				})
				So(len(output[11]), ShouldEqual, 6)
				So(output[11], ShouldResemble, []data.Map{
					{"l": data.Int(4), "r": data.Int(5)},
					{"l": data.Int(4), "r": data.Int(6)},
					{"l": data.Int(5), "r": data.Int(5)},
					{"l": data.Int(5), "r": data.Int(6)},
					{"l": data.Int(6), "r": data.Int(5)},
					{"l": data.Int(6), "r": data.Int(6)},
				})
			})
		})
	})

	Convey("Given a JOIN selecting from left and right with different RSTREAM emitters and ranges", t, func() {
		tuples := getTuples(12)
		// rearrange the tuples
		for i, t := range tuples {
			if i%2 == 0 {
				t.InputName = "src1"
				t.Data["a"] = data.Int(i/2 + 1)
			} else {
				t.InputName = "src2"
				t.Data["b"] = data.Int(i/2 + 1)
			}
		}
		s := `CREATE STREAM box AS SELECT
		RSTREAM [EVERY 3 TUPLES IN src2]
			x:a AS l, y:b AS r
		FROM src1 [RANGE 3 TUPLES] AS x, src2 [RANGE 2 TUPLES] AS y`

		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				// sort the output by the "l" and then the "r" key before
				// checking if it resembles the expected value
				sort.Sort(tupleList(out))
				output = append(output, out)
			}

			Convey("Then joined values should appear", func() {
				So(len(output), ShouldEqual, 12)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 0) // skip
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 6)
				So(output[5], ShouldResemble, []data.Map{
					{"l": data.Int(1), "r": data.Int(2)},
					{"l": data.Int(1), "r": data.Int(3)},
					{"l": data.Int(2), "r": data.Int(2)},
					{"l": data.Int(2), "r": data.Int(3)},
					{"l": data.Int(3), "r": data.Int(2)},
					{"l": data.Int(3), "r": data.Int(3)},
				})
				So(len(output[6]), ShouldEqual, 0)  // skip
				So(len(output[7]), ShouldEqual, 0)  // skip
				So(len(output[8]), ShouldEqual, 0)  // skip
				So(len(output[9]), ShouldEqual, 0)  // skip
				So(len(output[10]), ShouldEqual, 0) // skip
				So(len(output[11]), ShouldEqual, 6)
				So(output[11], ShouldResemble, []data.Map{
					{"l": data.Int(4), "r": data.Int(5)},
					{"l": data.Int(4), "r": data.Int(6)},
					{"l": data.Int(5), "r": data.Int(5)},
					{"l": data.Int(5), "r": data.Int(6)},
					{"l": data.Int(6), "r": data.Int(5)},
					{"l": data.Int(6), "r": data.Int(6)},
				})
			})
		})
	})

	Convey("Given a JOIN selecting from left and right with different ISTREAM emitters and ranges", t, func() {
		tuples := getTuples(12)
		// rearrange the tuples
		for i, t := range tuples {
			if i%2 == 0 {
				t.InputName = "src1"
				t.Data["a"] = data.Int(i/2 + 1)
			} else {
				t.InputName = "src2"
				t.Data["b"] = data.Int(i/2 + 1)
			}
		}
		s := `CREATE STREAM box AS SELECT
		ISTREAM [EVERY 2 TUPLES IN src1, 3 TUPLES IN src2]
			x:a AS l, y:b AS r
		FROM src1 [RANGE 3 TUPLES] AS x, src2 [RANGE 2 TUPLES] AS y`

		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				// sort the output by the "l" and then the "r" key before
				// checking if it resembles the expected value
				sort.Sort(tupleList(out))
				output = append(output, out)
			}

			Convey("Then joined values should appear", func() {
				So(len(output), ShouldEqual, 12)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 2)
				So(output[2], ShouldResemble, []data.Map{
					{"l": data.Int(1), "r": data.Int(1)},
					{"l": data.Int(2), "r": data.Int(1)},
				})
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 6)
				So(output[5], ShouldResemble, []data.Map{
					{"l": data.Int(1), "r": data.Int(2)},
					{"l": data.Int(1), "r": data.Int(3)},
					{"l": data.Int(2), "r": data.Int(2)},
					{"l": data.Int(2), "r": data.Int(3)},
					{"l": data.Int(3), "r": data.Int(2)},
					{"l": data.Int(3), "r": data.Int(3)},
				})
				So(len(output[6]), ShouldEqual, 2)
				So(output[6], ShouldResemble, []data.Map{
					{"l": data.Int(4), "r": data.Int(2)},
					{"l": data.Int(4), "r": data.Int(3)},
				})
				So(len(output[7]), ShouldEqual, 0) // skip
				So(len(output[8]), ShouldEqual, 0) // skip
				So(len(output[9]), ShouldEqual, 0) // skip
				So(len(output[10]), ShouldEqual, 6)
				So(output[10], ShouldResemble, []data.Map{
					{"l": data.Int(4), "r": data.Int(4)},
					{"l": data.Int(4), "r": data.Int(5)},
					{"l": data.Int(5), "r": data.Int(4)},
					{"l": data.Int(5), "r": data.Int(5)},
					{"l": data.Int(6), "r": data.Int(4)},
					{"l": data.Int(6), "r": data.Int(5)},
				})
				So(len(output[11]), ShouldEqual, 3)
				So(output[11], ShouldResemble, []data.Map{
					{"l": data.Int(4), "r": data.Int(6)},
					{"l": data.Int(5), "r": data.Int(6)},
					{"l": data.Int(6), "r": data.Int(6)},
				})
			})
		})
	})

	Convey("Given a JOIN selecting from left and right with different DSTREAM emitters and ranges", t, func() {
		tuples := getTuples(12)
		// rearrange the tuples
		for i, t := range tuples {
			if i%2 == 0 {
				t.InputName = "src1"
				t.Data["a"] = data.Int(i/2 + 1)
			} else {
				t.InputName = "src2"
				t.Data["b"] = data.Int(i/2 + 1)
			}
		}
		s := `CREATE STREAM box AS SELECT
		DSTREAM [EVERY 2 TUPLES IN src1, 3 TUPLES IN src2]
			x:a AS l, y:b AS r
		FROM src1 [RANGE 3 TUPLES] AS x, src2 [RANGE 2 TUPLES] AS y`

		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			output := [][]data.Map{}
			for _, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				// sort the output by the "l" and then the "r" key before
				// checking if it resembles the expected value
				sort.Sort(tupleList(out))
				output = append(output, out)
			}

			Convey("Then joined values should appear", func() {
				So(len(output), ShouldEqual, 12)
				So(len(output[0]), ShouldEqual, 0) // skip
				So(len(output[1]), ShouldEqual, 0) // skip
				So(len(output[2]), ShouldEqual, 0)
				So(len(output[3]), ShouldEqual, 0) // skip
				So(len(output[4]), ShouldEqual, 0) // skip
				So(len(output[5]), ShouldEqual, 2)
				So(output[5], ShouldResemble, []data.Map{
					{"l": data.Int(1), "r": data.Int(1)},
					{"l": data.Int(2), "r": data.Int(1)},
				})
				So(len(output[6]), ShouldEqual, 2)
				So(output[6], ShouldResemble, []data.Map{
					{"l": data.Int(1), "r": data.Int(2)},
					{"l": data.Int(1), "r": data.Int(3)},
				})
				So(len(output[7]), ShouldEqual, 0) // skip
				So(len(output[8]), ShouldEqual, 0) // skip
				So(len(output[9]), ShouldEqual, 0) // skip
				So(len(output[10]), ShouldEqual, 6)
				So(output[10], ShouldResemble, []data.Map{
					{"l": data.Int(2), "r": data.Int(2)},
					{"l": data.Int(2), "r": data.Int(3)},
					{"l": data.Int(3), "r": data.Int(2)},
					{"l": data.Int(3), "r": data.Int(3)},
					{"l": data.Int(4), "r": data.Int(2)},
					{"l": data.Int(4), "r": data.Int(3)},
				})
				So(len(output[11]), ShouldEqual, 3)
				So(output[11], ShouldResemble, []data.Map{
					{"l": data.Int(4), "r": data.Int(4)},
					{"l": data.Int(5), "r": data.Int(4)},
					{"l": data.Int(6), "r": data.Int(4)},
				})
			})
		})
	})

	Convey("Given a JOIN selecting from left and right with a join condition", t, func() {
		tuples := getTuples(8)
		// rearrange the tuples
		for i, t := range tuples {
			if i%2 == 0 {
				t.InputName = "src1"
				t.Data["l"] = data.String(fmt.Sprintf("l%d", i))
			} else {
				t.InputName = "src2"
				t.Data["r"] = data.String(fmt.Sprintf("r%d", i))
			}
		}
		s := `CREATE STREAM box AS SELECT ISTREAM src1:l, src2:r FROM src1 [RANGE 2 TUPLES], src2 [RANGE 2 TUPLES] ` +
			`WHERE src1:int + 1 = src2:int`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				// sort the output by the "l" and then the "r" key before
				// checking if it resembles the expected value
				sort.Sort(tupleList(out))

				Convey(fmt.Sprintf("Then joined values should appear in %v", idx), func() {
					if idx == 0 {
						So(len(out), ShouldEqual, 0)
					} else if idx == 1 {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String("l0"), // int: 1
							"r": data.String("r1"), // int: 2
						})
					} else if idx == 2 {
						So(len(out), ShouldEqual, 0)
					} else if idx%2 == 1 {
						// a tuple from src2 (=right) was just added
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String(fmt.Sprintf("l%d", idx-1)), // int: x
							"r": data.String(fmt.Sprintf("r%d", idx)),   // int: x+1
						})
					} else {
						// a tuple from src1 (=left) was just added
						So(len(out), ShouldEqual, 0)
					}
				})
			}
		})
	})

	Convey("Given a self-join with a join condition", t, func() {
		tuples := getTuples(8)
		// rearrange the tuples
		for i, t := range tuples {
			t.InputName = "src"
			t.Data["x"] = data.String(fmt.Sprintf("x%d", i))
		}
		s := `CREATE STREAM box AS SELECT ISTREAM src1:x AS l, src2:x AS r ` +
			`FROM src [RANGE 2 TUPLES] AS src1, src [RANGE 2 TUPLES] AS src2 ` +
			`WHERE src1:int + 1 = src2:int`
		plan, err := createGroupbyPlan(s, t)
		So(err, ShouldBeNil)

		Convey("When feeding it with tuples", func() {
			for idx, inTup := range tuples {
				out, err := plan.Process(inTup)
				So(err, ShouldBeNil)
				// sort the output by the "l" and then the "r" key before
				// checking if it resembles the expected value
				sort.Sort(tupleList(out))

				Convey(fmt.Sprintf("Then joined values should appear in %v", idx), func() {
					if idx == 0 {
						// join condition fails
						So(len(out), ShouldEqual, 0)
					} else {
						So(len(out), ShouldEqual, 1)
						So(out[0], ShouldResemble, data.Map{
							"l": data.String(fmt.Sprintf("x%d", idx-1)), // int: x
							"r": data.String(fmt.Sprintf("x%d", idx)),   // int: x+1
						})
					}
				})
			}
		})
	})
}
