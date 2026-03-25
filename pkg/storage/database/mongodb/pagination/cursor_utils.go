package pagination
import (
	"encoding/base64"
	"encoding/json"
	"time"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)
func ParseMongoCursor(cursor string) (bson.M, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	converted := bson.M{}
	for key, value := range parsed {
		if strValue, ok := value.(string); ok {
			if len(strValue) == 24 {
				if objectID, err := primitive.ObjectIDFromHex(strValue); err == nil {
					converted[key] = objectID
					continue
				}
			}
			if t, err := time.Parse(time.RFC3339, strValue); err == nil {
				converted[key] = t
				continue
			}
			converted[key] = strValue
		} else {
			converted[key] = value
		}
	}
	return converted, nil
}
func CreateMongoCursor(payload bson.M) (string, error) {
	serializable := make(map[string]interface{})
	for key, value := range payload {
		switch v := value.(type) {
		case primitive.ObjectID:
			serializable[key] = v.Hex()
		case time.Time:
			serializable[key] = v.Format(time.RFC3339)
		default:
			serializable[key] = v
		}
	}
	data, err := json.Marshal(serializable)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}