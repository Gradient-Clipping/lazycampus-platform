// Copyright (C) 2023-2026 QuantumNous. SPDX-License-Identifier: AGPL-3.0-or-later
package common

import (
	"encoding/json"
	"io"
)

func Marshal(v any) ([]byte, error)       { return json.Marshal(v) }
func Unmarshal(data []byte, v any) error  { return json.Unmarshal(data, v) }
func DecodeJson(r io.Reader, v any) error { return json.NewDecoder(r).Decode(v) }
