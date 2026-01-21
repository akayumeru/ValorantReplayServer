package utils

import (
	"encoding/json"
	"log"
)

func DebugLog(message string, context interface{}) {
	serializedContext, err := json.Marshal(context)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("%s: %s\n", message, string(serializedContext))
}
