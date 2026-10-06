#include "agl_stub.h"

#include <stddef.h>

enum {
    GL_FALSE = 0,
    AGL_NO_ERROR = 0,
    AGL_BAD_VALUE = 10008,
    AGL_BAD_ENUM = 10010,
    EXIT_CONFIGURE = 1,
    EXIT_DESTROY_PBUFFER = 2,
    EXIT_NOT_CLEARED = 3,
};

static int failed(GLboolean result, GLenum want) {
    if (result != GL_FALSE || aglGetError() != want) {
        return 1;
    }
    return 0;
}

int main(void) {
    if (failed(aglConfigure(0, 0), AGL_BAD_ENUM)) {
        return EXIT_CONFIGURE;
    }
    if (aglGetError() != AGL_NO_ERROR) {
        return EXIT_NOT_CLEARED;
    }
    if (failed(aglDestroyPBuffer(NULL), AGL_BAD_VALUE)) {
        return EXIT_DESTROY_PBUFFER;
    }
    return 0;
}
