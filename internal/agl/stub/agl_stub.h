#ifndef AGL_STUB_H
#define AGL_STUB_H

typedef unsigned int GLenum;
typedef int GLint;
typedef unsigned int GLuint;
typedef unsigned char GLboolean;
typedef unsigned char GLubyte;

typedef void *AGLPixelFormat;
typedef void *AGLContext;
typedef void *AGLDevice;
typedef void *AGLDrawable;
typedef void *AGLRendererInfo;
typedef void *AGLPbuffer;

/* Stub of the removed AGL API. Pixel-format and context creation succeed and return sentinel
 * handles; destroy, update, set-current and set-drawable return GL_TRUE. Every other operation
 * returns GL_FALSE, NULL or zero, zeroes its output parameters and, where it reports failure, sets
 * the error that aglGetError returns. */
AGLPixelFormat aglChoosePixelFormat(const void *gdevs, GLint ndev, const GLint *attribs);
void aglDestroyPixelFormat(AGLPixelFormat pix);
AGLPixelFormat aglNextPixelFormat(AGLPixelFormat pix);
GLboolean aglDescribePixelFormat(AGLPixelFormat pix, GLint attrib, GLint *value);
AGLDevice *aglDevicesOfPixelFormat(AGLPixelFormat pix, GLint *ndevs);
AGLRendererInfo aglQueryRendererInfo(const AGLDevice *gdevs, GLint ndev);
void aglDestroyRendererInfo(AGLRendererInfo rend);
AGLRendererInfo aglNextRendererInfo(AGLRendererInfo rend);
GLboolean aglDescribeRenderer(AGLRendererInfo rend, GLint prop, GLint *value);
AGLContext aglCreateContext(AGLPixelFormat pix, AGLContext share);
GLboolean aglDestroyContext(AGLContext ctx);
GLboolean aglCopyContext(AGLContext src, AGLContext dst, GLuint mask);
GLboolean aglUpdateContext(AGLContext ctx);
GLboolean aglSetCurrentContext(AGLContext ctx);
AGLContext aglGetCurrentContext(void);
GLboolean aglSetDrawable(AGLContext ctx, AGLDrawable draw);
GLboolean aglSetFullScreen(AGLContext ctx, GLint width, GLint height, GLint freq, GLint device);
AGLDrawable aglGetDrawable(AGLContext ctx);
GLboolean aglSetVirtualScreen(AGLContext ctx, GLint screen);
GLint aglGetVirtualScreen(AGLContext ctx);
GLboolean aglSetOffScreen(AGLContext ctx, GLint width, GLint height, GLint rowbytes,
                          void *baseaddr);
GLboolean aglGetOffScreen(AGLContext ctx, GLint *width, GLint *height, GLint *rowbytes,
                          void **baseaddr);
GLboolean aglEnable(AGLContext ctx, GLenum pname);
GLboolean aglDisable(AGLContext ctx, GLenum pname);
GLboolean aglIsEnabled(AGLContext ctx, GLenum pname);
GLboolean aglSetInteger(AGLContext ctx, GLenum pname, const GLint *params);
GLboolean aglGetInteger(AGLContext ctx, GLenum pname, GLint *params);
GLboolean aglUseFont(AGLContext ctx, GLint fontID, GLint face, GLint size, GLint first, GLint count,
                     GLint base);
GLenum aglGetError(void);
const GLubyte *aglErrorString(GLenum code);
void aglSwapBuffers(AGLContext ctx);
GLboolean aglConfigure(GLenum pname, GLuint param);
void aglResetLibrary(void);
GLboolean aglCreatePBuffer(GLint width, GLint height, GLenum target, GLenum internalFormat,
                           long max_level, AGLPbuffer *pbuffer);
GLboolean aglDestroyPBuffer(AGLPbuffer pbuffer);
GLboolean aglDescribePBuffer(AGLPbuffer pbuffer, GLint *width, GLint *height, GLenum *target,
                             GLenum *internalFormat, GLint *max_level);
GLboolean aglTexImagePBuffer(AGLContext ctx, AGLPbuffer pbuffer, GLint source);
GLboolean aglSetPBuffer(AGLContext ctx, AGLPbuffer pbuffer, GLint face, GLint level, GLint screen);
GLboolean aglGetPBuffer(AGLContext ctx, AGLPbuffer *pbuffer, GLint *face, GLint *level,
                        GLint *screen);
GLboolean aglGetCGLContext(AGLContext ctx, void **cgl_ctx);
GLboolean aglGetCGLPixelFormat(AGLPixelFormat pix, void **cgl_pix);

#endif
