package securedload

import "path/filepath"

func GetAssetSecured(name string) ([]byte, error) {
	name = filepath.FromSlash(name)

	var err error
	for k, v := range knownProtectedLoader {
		loadedData, errLoad := v.VerifyAndLoad(name)
		if errLoad == nil {
			return loadedData, nil
		}
		err = newError(k, " is not loading executable file").Base(errLoad)
	}
	return nil, err
}
