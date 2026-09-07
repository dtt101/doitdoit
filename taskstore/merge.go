package taskstore

import (
	"fmt"
	"reflect"
)

func Merge(base, local, remote Data) (Data, error) {
	merged := Clone(remote)
	keys := make(map[string]struct{}, len(base)+len(local)+len(remote))
	for key := range base {
		keys[key] = struct{}{}
	}
	for key := range local {
		keys[key] = struct{}{}
	}
	for key := range remote {
		keys[key] = struct{}{}
	}
	for key := range keys {
		baseTasks, baseOK := base[key]
		localTasks, localOK := local[key]
		remoteTasks, remoteOK := remote[key]
		localChanged := localOK != baseOK || !reflect.DeepEqual(localTasks, baseTasks)
		remoteChanged := remoteOK != baseOK || !reflect.DeepEqual(remoteTasks, baseTasks)
		switch {
		case !localChanged:
			continue
		case !remoteChanged || (localOK == remoteOK && reflect.DeepEqual(localTasks, remoteTasks)):
			if localOK {
				merged[key] = append([]Task(nil), localTasks...)
			} else {
				delete(merged, key)
			}
		default:
			return nil, fmt.Errorf("%w: both copies changed %s", ErrDataConflict, key)
		}
	}
	return merged, nil
}
