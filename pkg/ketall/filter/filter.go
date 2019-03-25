/*
Copyright 2019 Cornelius Weig

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package filter

import (
	"regexp"
	"strconv"
	"time"

	"github.com/corneliusweig/ketall/pkg/ketall/constants"
	"github.com/corneliusweig/ketall/pkg/ketall/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

type Predicate = func(runtime.Object) bool

func ApplyFilter(o runtime.Object) runtime.Object {
	since := viper.GetString(constants.FlagSince)

	if since == "" {
		logrus.Debugf("No filter found")
		return o
	}
	logrus.Debugf("Found %s argument %s", constants.FlagSince, since)

	predicate, err := AgePredicate(since)
	if err != nil {
		logrus.Warnf("%s", errors.Wrapf(err, "skipping filter"))
		return o
	}

	filtered, err := ByPredicate(o, predicate)
	if err != nil {
		logrus.Warnf("%s", errors.Wrapf(err, "filtering failed"))
		return o
	}

	return filtered
}

func ByPredicate(o runtime.Object, p Predicate) (runtime.Object, error) {
	if !meta.IsListType(o) {
		if p(o) {
			return o, nil
		}
		return nil, nil
	}

	allItems, err := meta.ExtractList(o)
	if err != nil {
		return nil, errors.Wrap(err, "extract resource list")
	}

	var items []runtime.Object
	for _, item := range allItems {
		item, err := ByPredicate(item, p)
		if err != nil {
			return nil, err
		}
		if item != nil {
			items = append(items, item)
		}
	}

	return util.ToV1List(items), nil
}

func AgePredicate(since string) (Predicate, error) {
	duration, err := ParseHumanDuration(since)
	if err != nil {
		return nil, errors.Wrapf(err, "parse duration %s", since)
	}
	sinceTimestamp := time.Now().Add(-duration)

	return func(o runtime.Object) bool {
		acc, err := meta.Accessor(o)
		if err != nil {
			logrus.Warnf("could not extract object metadata for filter")
			return true
		}

		creationTimestamp := acc.GetCreationTimestamp().Time
		return !sinceTimestamp.After(creationTimestamp)
	}, nil
}

func ParseHumanDuration(since string) (time.Duration, error) {
	matchDuration := regexp.MustCompile(`(\d+y)?(\d+d)?(\d+h)?(\d+m)?(\d+s)?`)
	allMatches := matchDuration.FindAllStringSubmatch(since, -1)
	if len(allMatches) != 1 {
		return time.Duration(0), errors.Errorf("not a valid duration: '%s'", since)
	}

	var seconds int64
	for _, m := range allMatches[0][1:] {
		if m == "" {
			continue
		}
		unit := m[len(m)-1]
		value, _ := strconv.ParseInt(m[:len(m)-1], 10, 64)
		switch unit {
		case 'y':
			seconds += value * (365 * 24 * 60 * 60)
		case 'd':
			seconds += value * (24 * 60 * 60)
		case 'h':
			seconds += value * (60 * 60)
		case 'm':
			seconds += value * 60
		case 's':
			seconds += value
		default:
			return time.Duration(0), errors.Errorf("not a known unit: '%b'", unit)
		}
	}
	return time.Duration(int64(time.Second) * seconds), nil
}
