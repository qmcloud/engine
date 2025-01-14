## Overview

This folder has vendored packages utilized by the various graphics packages (namely the renderers). They are internal packages and should not be used by anyone else.

| Package         | Description                                                             |
|-----------------|-------------------------------------------------------------------------|
| gl/2.0/gl       | OpenGL 2.0 wrappers generated using Glow.                               |
| gles2/2.0/gles2 | OpenGL ES 2.0 wrappers generated using Glow.                            |
| restrict.json   | Glow symbol restriction JSON file.                                      |
| resize          | Appengine image resizing package.                                       |
| util            | Common gfx.Device utilities.                                            |
| glutil          | Standard OpenGL device utilities.                                       |
| tag             | Simply exposes a few build tags.                                        |
| glc             | Open(GL) (C)ommon, a shared set of OpenGL API's across OpenGL versions. |

## Glow

Glow (the OpenGL wrapper generator) can be found [on GitHub](http://github.com/go-gl/glow).

## Regenerating

To regenerate the bindings you must:

```
cd azul3d.org/gfx/internal
glow download
go generate
```
