package ecstaskstatechange

import (
    "encoding/json"
)

func Marshal(inputEvent interface{}) ([]byte, error) {
    outputStream, err := json.Marshal(inputEvent)
    if err != nil {
        return nil, err
    }

    return outputStream, nil
}

func Unmarshal(inputStream []byte) (map[string]interface{}, error) {
    var outputEvent map[string]interface{}
    err := json.Unmarshal(inputStream, &outputEvent)
    if err != nil {
        return nil, err
    }

    return outputEvent, nil
}

func UnmarshalEvent(inputStream []byte) (AWSEvent, error) {
    var outputEvent AWSEvent
    err := json.Unmarshal(inputStream, &outputEvent)
    if err != nil {
        return AWSEvent{}, err
    }

    return outputEvent, nil
}
