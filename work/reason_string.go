// Code generated by "stringer -type=Reason"; DO NOT EDIT

package work

import "fmt"

const _Reason_name = "NoFailurePreCheckFailureContCheckFailureMaxFailures"

var _Reason_index = [...]uint8{0, 9, 24, 40, 51}

func (i Reason) String() string {
	if i < 0 || i >= Reason(len(_Reason_index)-1) {
		return fmt.Sprintf("Reason(%d)", i)
	}
	return _Reason_name[_Reason_index[i]:_Reason_index[i+1]]
}
