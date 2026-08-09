package tools

import "fmt"

func parseEditOps(input any) ([]EditOp, error) {
	raw, ok := input.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid input: ops must be a non-empty array")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("invalid input: ops must be a non-empty array")
	}
	ops := make([]EditOp, len(raw))
	for i, item := range raw {
		op, err := parseEditOp(item)
		if err != nil {
			return nil, fmt.Errorf("op %d: %w", i+1, err)
		}
		ops[i] = op
	}
	return ops, nil
}

func parseEditOp(item any) (EditOp, error) {
	obj, ok := item.(map[string]any)
	if !ok {
		return EditOp{}, fmt.Errorf("each op must be an object")
	}

	op := EditOp{Op: opReplace}
	if v, exists := obj["op"]; exists {
		s, ok := v.(string)
		if !ok {
			return EditOp{}, fmt.Errorf("op must be a string")
		}
		op.Op = s
	}
	switch op.Op {
	case opReplace, opInsertAfter, opInsertBefore, opInsertHead, opInsertTail:
	default:
		return EditOp{}, fmt.Errorf("unsupported operation %q (expected one of: replace, insert_after, insert_before, insert_head, insert_tail)", op.Op)
	}

	if v, exists := obj["text"]; exists {
		s, ok := v.(string)
		if !ok {
			return EditOp{}, fmt.Errorf("text must be a string")
		}
		op.Text = s
	}

	start, err := editOpAnchor(obj, "start")
	if err != nil {
		return EditOp{}, err
	}
	end, err := editOpAnchor(obj, "end")
	if err != nil {
		return EditOp{}, err
	}

	switch op.Op {
	case opReplace, opInsertAfter, opInsertBefore:
		if start == "" {
			return EditOp{}, fmt.Errorf("operation %q requires a start anchor", op.Op)
		}
		op.Start = start
	case opInsertHead, opInsertTail:
		if start != "" {
			return EditOp{}, fmt.Errorf("operation %q takes no start anchor", op.Op)
		}
	}

	if op.Op == opReplace {
		if end != "" {
			op.End = end
		}
	} else if end != "" {
		return EditOp{}, fmt.Errorf("only replace accepts an end anchor (operation %q does not)", op.Op)
	}

	return op, nil
}

func editOpAnchor(obj map[string]any, name string) (string, error) {
	v, exists := obj[name]
	if !exists {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string anchor", name)
	}
	return s, nil
}

type validatedEditOp struct {
	op    EditOp
	start int
	end   int
}

func anchorPositions(lines [][]byte) map[string]int {
	anchors := make(map[string]int, len(lines))
	for i, line := range lines {
		anchors[fmt.Sprintf("%d:%s", i+1, computeLineHash(line))] = i + 1
	}
	return anchors
}

func validateAnchor(anchor string, anchors map[string]int, opNum int) (int, error) {
	line, ok := anchors[anchor]
	if !ok {
		return 0, fmt.Errorf("op %d: anchor %q does not exist in the current file snapshot; re-read the file and retry", opNum, anchor)
	}
	return line, nil
}

func validateEditOps(content []byte, ops []EditOp) ([]validatedEditOp, error) {
	lines := splitRawLines(content)
	anchors := anchorPositions(lines)
	validated := make([]validatedEditOp, 0, len(ops))
	for i, op := range ops {
		opNum := i + 1
		switch op.Op {
		case opInsertHead, opInsertTail:
			if len(lines) == 0 && op.Op == opInsertTail {
				return nil, fmt.Errorf("op %d: only insert_head is valid for an empty file", opNum)
			}
			start := 0
			if op.Op == opInsertTail {
				start = len(lines)
			}
			validated = append(validated, validatedEditOp{op: op, start: start, end: start})
		case opReplace:
			from, err := validateAnchor(op.Start, anchors, opNum)
			if err != nil {
				return nil, err
			}
			to := from
			if op.End != "" {
				if to, err = validateAnchor(op.End, anchors, opNum); err != nil {
					return nil, err
				}
				if to < from {
					from, to = to, from
				}
			}
			validated = append(validated, validatedEditOp{op: op, start: from - 1, end: to})
		default:
			line, err := validateAnchor(op.Start, anchors, opNum)
			if err != nil {
				return nil, err
			}
			start := line - 1
			if op.Op == opInsertAfter {
				start = line
			}
			validated = append(validated, validatedEditOp{op: op, start: start, end: start})
		}
	}
	for i := 0; i < len(validated); i++ {
		for j := i + 1; j < len(validated); j++ {
			if editOpsConflict(validated[i], validated[j]) {
				return nil, editConflictError(i+1, j+1, validated[i], validated[j])
			}
		}
	}
	return validated, nil
}

func editOpsConflict(a, b validatedEditOp) bool {
	if a.op.Op == opReplace && b.op.Op == opReplace {
		return a.start < b.end && b.start < a.end
	}
	if a.op.Op == opReplace || b.op.Op == opReplace {
		r, ins := a, b
		if b.op.Op == opReplace {
			r, ins = b, a
		}
		return r.start < ins.start && ins.start < r.end
	}
	return a.start == b.start
}

func editConflictError(i, j int, a, b validatedEditOp) error {
	if a.op.Op == opReplace && b.op.Op == opReplace {
		return fmt.Errorf("ops %d and %d have overlapping ranges (lines %d-%d and %d-%d)", i, j, a.start+1, a.end, b.start+1, b.end)
	}
	if a.op.Op == opReplace || b.op.Op == opReplace {
		r, ins, rNum, iNum := a, b, i, j
		if b.op.Op == opReplace {
			r, ins, rNum, iNum = b, a, j, i
		}
		line := ins.start
		if ins.op.Op == opInsertBefore {
			line++
		}
		return fmt.Errorf("ops %d and %d conflict: insertion at line %d overlaps replacement range (lines %d-%d)", rNum, iNum, line, r.start+1, r.end)
	}
	return fmt.Errorf("ops %d and %d conflict: both insert at the same position; combine them into one op", i, j)
}
