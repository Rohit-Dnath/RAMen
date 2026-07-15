package server

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Rohit-Dnath/RAMen/internal/store"
)

func (c *conn) cmdCopy(args []string) error {
	if len(args) < 3 || len(args) > 4 {
		return c.wrongArgs("copy")
	}

	src, dst := args[1], args[2]
	replace := len(args) == 4 && strings.ToUpper(args[3]) == "REPLACE"

	v, ok, err := c.s.store.Get(src)
	if err != nil {
		return c.storeErr(err)
	}
	if !ok {
		return c.storeErr(fmt.Errorf("invalid source to copy from"))
	}

	_, ok, err = c.s.store.Get(dst)
	if err != nil {
		return c.storeErr(err)
	}
	if ok && replace == false {
		return c.storeErr(fmt.Errorf("0"))
	}

	if c.s.store.Set(dst, v, store.SetOptions{}) {
		return c.writeSimple("1")
	}
	return c.writeNull()
}

func (c *conn) cmdObjectEncoding(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("object encoding")
	}

	// currently behaves similar to `type` command because store doesn't use different internal representation
	return c.writeSimple(c.s.store.Type(args[2]))
}

func orderSortArgs(args []string) []string {
	res := []string{}
	hasDesc := false

	for _, arg := range args {
		if strings.ToUpper(arg) == "DESC" {
			hasDesc = true
			continue
		}

		res = append(res, strings.ToUpper(arg))
	}

	if hasDesc {
		res = append([]string{"DESC"}, res...)
	}
	return res
}

func checkArray(vals []string) bool {
	for _, val := range vals {
		_, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
	}
	return true
}

func numsToString(nums []float64) []string {
	res := []string{}
	for _, num := range nums {
		res = append(res, strconv.FormatFloat(num, 'f', -1, 64))
	}
	return res
}

func (c *conn) cmdSort(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("sort")
	}

	items, err := c.s.store.LRange(args[1], 0, -1)
	if err != nil {
		return c.storeErr(err)
	}

	// since default sort is by number all item values have to be a number
	numSortable := checkArray(items)

	// order of execution for the args matters even though order layout maybe unordered
	// example: LIMIT start end DESC output is actually DESC LIMIT start end
	args = orderSortArgs(args[2:])

	isAlpha := slices.Contains(args, "ALPHA")
	if numSortable && !isAlpha {
		// only for internal purpose
		args = append([]string{"NUMERIC"}, args...)
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "NUMERIC":
			nums := []float64{}
			for _, val := range items {
				n, _ := strconv.ParseFloat(val, 64)
				nums = append(nums, n)
			}

			sort.Slice(nums, func(i, j int) bool {
				return nums[i] < nums[j]
			})

			items = numsToString(nums)
		case "ALPHA":
			slices.Sort(items)
		case "DESC":
			slices.Reverse(items)
		case "LIMIT":
			if i+2 >= len(args) {
				return c.wrongArgs("sort limit")
			}
			offset, err := strconv.Atoi(args[i+1])
			if err != nil {
				return c.writeError("invalid limit index " + args[i+1])
			}
			count, err := strconv.Atoi(args[i+2])
			if err != nil {
				return c.writeError("invalid limit index " + args[i+2])
			}

			if offset >= len(items) {
				items = nil
			} else {
				if count < 0 {
					count = len(items) - offset
				}

				end := min(offset+count, len(items))
				items = items[offset:end]
			}
			i += 2
		case "ASC":
		default:
			return c.writeError(fmt.Sprintf("syntax error, %s", strings.ToUpper(args[i])))
		}
	}

	return c.writeStringArray(items)
}
