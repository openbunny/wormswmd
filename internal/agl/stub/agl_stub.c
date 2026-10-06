#include "agl_stub.h"

#include <stddef.h>

enum {
    GL_FALSE = 0,
    GL_TRUE = 1,
    PIXEL_FORMAT_VALUE = 32,
    AGL_NO_VIRTUAL_SCREEN = 0,
};

enum {
    AGL_NO_ERROR = 0,
    AGL_BAD_ATTRIBUTE = 10000,
    AGL_BAD_PROPERTY = 10001,
    AGL_BAD_PIXELFMT = 10002,
    AGL_BAD_RENDINFO = 10003,
    AGL_BAD_CONTEXT = 10004,
    AGL_BAD_DRAWABLE = 10005,
    AGL_BAD_GDEV = 10006,
    AGL_BAD_STATE = 10007,
    AGL_BAD_VALUE = 10008,
    AGL_BAD_MATCH = 10009,
    AGL_BAD_ENUM = 10010,
    AGL_BAD_OFFSCREEN = 10011,
    AGL_BAD_FULLSCREEN = 10012,
    AGL_BAD_WINDOW = 10013,
    AGL_BAD_POINTER = 10014,
    AGL_BAD_MODULE = 10015,
    AGL_BAD_ALLOC = 10016,
    AGL_BAD_CONNECTION = 10017,
};

/* agl_last_error is process-global. AGL callers serialize these calls. */
static GLenum agl_last_error = AGL_NO_ERROR;
static int agl_format;
static int agl_context;

static void fail(GLenum code) { agl_last_error = code; }

static void zero_int(GLint *out) {
    if (out != NULL) {
        *out = 0;
    }
}

static void zero_enum(GLenum *out) {
    if (out != NULL) {
        *out = 0;
    }
}

static void zero_ptr(void **out) {
    if (out != NULL) {
        *out = NULL;
    }
}

AGLPixelFormat aglChoosePixelFormat(const void *gdevs, GLint ndev, const GLint *attribs) {
    (void)gdevs;
    (void)ndev;
    (void)attribs;
    agl_last_error = AGL_NO_ERROR;
    return &agl_format;
}

void aglDestroyPixelFormat(AGLPixelFormat pix) { (void)pix; }

AGLPixelFormat aglNextPixelFormat(AGLPixelFormat pix) {
    (void)pix;
    return NULL;
}

GLboolean aglDescribePixelFormat(AGLPixelFormat pix, GLint attrib, GLint *value) {
    (void)pix;
    (void)attrib;
    if (value != NULL) {
        *value = PIXEL_FORMAT_VALUE;
    }
    agl_last_error = AGL_NO_ERROR;
    return GL_TRUE;
}

AGLDevice *aglDevicesOfPixelFormat(AGLPixelFormat pix, GLint *ndevs) {
    (void)pix;
    zero_int(ndevs);
    return NULL;
}

AGLRendererInfo aglQueryRendererInfo(const AGLDevice *gdevs, GLint ndev) {
    (void)gdevs;
    (void)ndev;
    fail(AGL_BAD_CONTEXT);
    return NULL;
}

void aglDestroyRendererInfo(AGLRendererInfo rend) { (void)rend; }

AGLRendererInfo aglNextRendererInfo(AGLRendererInfo rend) {
    (void)rend;
    return NULL;
}

GLboolean aglDescribeRenderer(AGLRendererInfo rend, GLint prop, GLint *value) {
    (void)rend;
    (void)prop;
    zero_int(value);
    fail(AGL_BAD_RENDINFO);
    return GL_FALSE;
}

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
AGLContext aglCreateContext(AGLPixelFormat pix, AGLContext share) {
    (void)share;
    if (pix == NULL) {
        fail(AGL_BAD_PIXELFMT);
        return NULL;
    }
    agl_last_error = AGL_NO_ERROR;
    return &agl_context;
}

GLboolean aglDestroyContext(AGLContext ctx) {
    (void)ctx;
    agl_last_error = AGL_NO_ERROR;
    return GL_TRUE;
}

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
GLboolean aglCopyContext(AGLContext src, AGLContext dst, GLuint mask) {
    (void)src;
    (void)dst;
    (void)mask;
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLboolean aglUpdateContext(AGLContext ctx) {
    (void)ctx;
    agl_last_error = AGL_NO_ERROR;
    return GL_TRUE;
}

GLboolean aglSetCurrentContext(AGLContext ctx) {
    (void)ctx;
    agl_last_error = AGL_NO_ERROR;
    return GL_TRUE;
}

AGLContext aglGetCurrentContext(void) { return NULL; }

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
GLboolean aglSetDrawable(AGLContext ctx, AGLDrawable draw) {
    (void)ctx;
    (void)draw;
    agl_last_error = AGL_NO_ERROR;
    return GL_TRUE;
}

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
GLboolean aglSetFullScreen(AGLContext ctx, GLint width, GLint height, GLint freq, GLint device) {
    (void)ctx;
    (void)width;
    (void)height;
    (void)freq;
    (void)device;
    fail(AGL_BAD_FULLSCREEN);
    return GL_FALSE;
}

AGLDrawable aglGetDrawable(AGLContext ctx) {
    (void)ctx;
    return NULL;
}

GLboolean aglSetVirtualScreen(AGLContext ctx, GLint screen) {
    (void)ctx;
    (void)screen;
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLint aglGetVirtualScreen(AGLContext ctx) {
    (void)ctx;
    return AGL_NO_VIRTUAL_SCREEN;
}

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
GLboolean aglSetOffScreen(AGLContext ctx, GLint width, GLint height, GLint rowbytes,
                          void *baseaddr) {
    (void)ctx;
    (void)width;
    (void)height;
    (void)rowbytes;
    (void)baseaddr;
    fail(AGL_BAD_OFFSCREEN);
    return GL_FALSE;
}

GLboolean aglGetOffScreen(AGLContext ctx, GLint *width, GLint *height, GLint *rowbytes,
                          void **baseaddr) {
    (void)ctx;
    zero_int(width);
    zero_int(height);
    zero_int(rowbytes);
    zero_ptr(baseaddr);
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLboolean aglEnable(AGLContext ctx, GLenum pname) {
    (void)ctx;
    (void)pname;
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLboolean aglDisable(AGLContext ctx, GLenum pname) {
    (void)ctx;
    (void)pname;
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLboolean aglIsEnabled(AGLContext ctx, GLenum pname) {
    (void)ctx;
    (void)pname;
    return GL_FALSE;
}

GLboolean aglSetInteger(AGLContext ctx, GLenum pname, const GLint *params) {
    (void)ctx;
    (void)pname;
    (void)params;
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLboolean aglGetInteger(AGLContext ctx, GLenum pname, GLint *params) {
    (void)ctx;
    (void)pname;
    zero_int(params);
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
GLboolean aglUseFont(AGLContext ctx, GLint fontID, GLint face, GLint size, GLint first, GLint count,
                     GLint base) {
    (void)ctx;
    (void)fontID;
    (void)face;
    (void)size;
    (void)first;
    (void)count;
    (void)base;
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLenum aglGetError(void) {
    GLenum code = agl_last_error;
    agl_last_error = AGL_NO_ERROR;
    return code;
}

const GLubyte *aglErrorString(GLenum code) {
    switch (code) {
    case AGL_NO_ERROR:
        return (const GLubyte *)"No error";
    case AGL_BAD_ATTRIBUTE:
        return (const GLubyte *)"Bad attribute";
    case AGL_BAD_PROPERTY:
        return (const GLubyte *)"Bad property";
    case AGL_BAD_PIXELFMT:
        return (const GLubyte *)"Bad pixel format";
    case AGL_BAD_RENDINFO:
        return (const GLubyte *)"Bad renderer info";
    case AGL_BAD_CONTEXT:
        return (const GLubyte *)"Bad context";
    case AGL_BAD_DRAWABLE:
        return (const GLubyte *)"Bad drawable";
    case AGL_BAD_GDEV:
        return (const GLubyte *)"Bad graphics device";
    case AGL_BAD_STATE:
        return (const GLubyte *)"Bad state";
    case AGL_BAD_VALUE:
        return (const GLubyte *)"Bad value";
    case AGL_BAD_MATCH:
        return (const GLubyte *)"Bad match";
    case AGL_BAD_ENUM:
        return (const GLubyte *)"Bad enum";
    case AGL_BAD_OFFSCREEN:
        return (const GLubyte *)"Bad offscreen";
    case AGL_BAD_FULLSCREEN:
        return (const GLubyte *)"Bad fullscreen";
    case AGL_BAD_WINDOW:
        return (const GLubyte *)"Bad window";
    case AGL_BAD_POINTER:
        return (const GLubyte *)"Bad pointer";
    case AGL_BAD_MODULE:
        return (const GLubyte *)"Bad module";
    case AGL_BAD_ALLOC:
        return (const GLubyte *)"Bad alloc";
    case AGL_BAD_CONNECTION:
        return (const GLubyte *)"Bad connection";
    default:
        return (const GLubyte *)"Unknown error";
    }
}

void aglSwapBuffers(AGLContext ctx) { (void)ctx; }

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
GLboolean aglConfigure(GLenum pname, GLuint param) {
    (void)pname;
    (void)param;
    fail(AGL_BAD_ENUM);
    return GL_FALSE;
}

void aglResetLibrary(void) { agl_last_error = AGL_NO_ERROR; }

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
GLboolean aglCreatePBuffer(GLint width, GLint height, GLenum target, GLenum internalFormat,
                           long max_level, AGLPbuffer *pbuffer) {
    (void)width;
    (void)height;
    (void)target;
    (void)internalFormat;
    (void)max_level;
    zero_ptr(pbuffer);
    fail(AGL_BAD_ALLOC);
    return GL_FALSE;
}

GLboolean aglDestroyPBuffer(AGLPbuffer pbuffer) {
    (void)pbuffer;
    fail(AGL_BAD_VALUE);
    return GL_FALSE;
}

GLboolean aglDescribePBuffer(AGLPbuffer pbuffer, GLint *width, GLint *height, GLenum *target,
                             GLenum *internalFormat, GLint *max_level) {
    (void)pbuffer;
    zero_int(width);
    zero_int(height);
    zero_enum(target);
    zero_enum(internalFormat);
    zero_int(max_level);
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
GLboolean aglTexImagePBuffer(AGLContext ctx, AGLPbuffer pbuffer, GLint source) {
    (void)ctx;
    (void)pbuffer;
    (void)source;
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

/* NOLINTNEXTLINE(bugprone-easily-swappable-parameters): the AGL ABI fixes this signature. */
GLboolean aglSetPBuffer(AGLContext ctx, AGLPbuffer pbuffer, GLint face, GLint level, GLint screen) {
    (void)ctx;
    (void)pbuffer;
    (void)face;
    (void)level;
    (void)screen;
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLboolean aglGetPBuffer(AGLContext ctx, AGLPbuffer *pbuffer, GLint *face, GLint *level,
                        GLint *screen) {
    (void)ctx;
    zero_ptr(pbuffer);
    zero_int(face);
    zero_int(level);
    zero_int(screen);
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLboolean aglGetCGLContext(AGLContext ctx, void **cgl_ctx) {
    (void)ctx;
    zero_ptr(cgl_ctx);
    fail(AGL_BAD_CONTEXT);
    return GL_FALSE;
}

GLboolean aglGetCGLPixelFormat(AGLPixelFormat pix, void **cgl_pix) {
    (void)pix;
    zero_ptr(cgl_pix);
    fail(AGL_BAD_PIXELFMT);
    return GL_FALSE;
}
