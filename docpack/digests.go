// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import "sort"

// TopicDigest is one topic's identity and content digest.
type TopicDigest struct {
	Key    string
	Locale string
	File   string
	Digest string
}

// Digests reports the content digest of every topic in the inputs.
//
// The review gate compares an approval against a digest, and the
// translation check compares a translation against one. Both values have
// to be *recorded in the source* by whoever approves or translates —
// which means the authoring layer (trendvidia/goed#321) and anyone
// hand-editing a topic needs a way to compute what the compiler will
// compute. Without this, the canonical encoding would be knowable only
// by reading the compiler's source, and a gate nobody can satisfy is a
// gate everybody routes around.
func Digests(inputs []string) ([]TopicDigest, []Diagnostic, error) {
	d := &diags{}
	sources, err := loadSources(inputs, nil, d)
	if err != nil {
		return nil, d.sorted(), err
	}
	var out []TopicDigest
	for _, src := range sources {
		for _, msg := range src.File.msgs("topics") {
			key, locale := msg.str("key"), msg.str("locale")
			digest, err := contentDigest(msg)
			if err != nil {
				d.errorf(Loc{File: src.Rel, Topic: key}, "computing content digest: %v", err)
				continue
			}
			out = append(out, TopicDigest{Key: key, Locale: locale, File: src.Rel, Digest: digest})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Locale < out[j].Locale
	})
	return out, d.sorted(), nil
}
