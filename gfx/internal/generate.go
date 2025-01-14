//go:generate rm -rf ./gles2
//go:generate glow generate -out=./gles2/2.0/gles2/ -api=gles2 -version=2.0 -restrict=./restrict.json
//go:generate rm -rf ./gl
//go:generate glow generate -out=./gl/2.0/gl/ -api=gl -version=2.0 -restrict=./restrict.json

package internal
