// Copyright (C) 2026 Luke Holland
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav_test

import (
	"fmt"
	"log"

	"github.com/LukeHollandDev/palworld-live-map/internal/palsav"
)

func ExampleLoad() {
	save, err := palsav.Load("LevelMeta.sav")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(save.Header.ClassName)
	for _, property := range save.Properties {
		fmt.Printf("%s (%s): %#v\n", property.Name, property.Type, property.Value)
	}
}

func ExampleStructArray_Iterator() {
	save, err := palsav.Load("player_dps.sav")
	if err != nil {
		log.Fatal(err)
	}

	property := save.Properties.Find("SaveParameterArray")
	array := property.Value.(palsav.ArrayValue).Structs
	iterator := array.Iterator()
	for iterator.Next() {
		properties := iterator.Value().(palsav.Properties)
		_ = properties
	}
	if err := iterator.Err(); err != nil {
		log.Fatal(err)
	}
}
