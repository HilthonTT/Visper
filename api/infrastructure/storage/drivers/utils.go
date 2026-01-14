package drivers

import "github.com/hilthontt/visper/api/infrastructure/model"

type UpdateProgress = model.UpdateProgress

type Progress struct {
	Total int64
	Done  int64
	up    UpdateProgress
}
