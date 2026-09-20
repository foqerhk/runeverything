//go:build windows

/*
  DXGI Desktop Duplication in C (COM vtable).
  File suffix _windows.c → only built on Windows.
*/
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#define COBJMACROS
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <d3d11.h>
#include <dxgi1_2.h>

static ID3D11Device *g_dev;
static ID3D11DeviceContext *g_ctx;
static IDXGIOutputDuplication *g_dup;
static ID3D11Texture2D *g_staging;
static int g_w, g_h;

static void re_dxgi_release_all(void) {
	if (g_staging) { ID3D11Texture2D_Release(g_staging); g_staging = NULL; }
	if (g_dup) { IDXGIOutputDuplication_Release(g_dup); g_dup = NULL; }
	if (g_ctx) { ID3D11DeviceContext_Release(g_ctx); g_ctx = NULL; }
	if (g_dev) { ID3D11Device_Release(g_dev); g_dev = NULL; }
	g_w = g_h = 0;
}

int re_dxgi_open(int output_index, int *w, int *h) {
	re_dxgi_release_all();
	D3D_FEATURE_LEVEL fl;
	HRESULT hr = D3D11CreateDevice(NULL, D3D_DRIVER_TYPE_HARDWARE, NULL, 0, NULL, 0,
		D3D11_SDK_VERSION, &g_dev, &fl, &g_ctx);
	if (FAILED(hr) || !g_dev) return -1;

	IDXGIDevice *dxgiDev = NULL;
	hr = ID3D11Device_QueryInterface(g_dev, &IID_IDXGIDevice, (void**)&dxgiDev);
	if (FAILED(hr)) { re_dxgi_release_all(); return -2; }

	IDXGIAdapter *adapter = NULL;
	hr = IDXGIDevice_GetAdapter(dxgiDev, &adapter);
	IDXGIDevice_Release(dxgiDev);
	if (FAILED(hr)) { re_dxgi_release_all(); return -3; }

	IDXGIOutput *output = NULL;
	UINT idx = output_index < 0 ? 0u : (UINT)output_index;
	hr = IDXGIAdapter_EnumOutputs(adapter, idx, &output);
	if (FAILED(hr)) hr = IDXGIAdapter_EnumOutputs(adapter, 0, &output);
	IDXGIAdapter_Release(adapter);
	if (FAILED(hr) || !output) { re_dxgi_release_all(); return -4; }

	DXGI_OUTPUT_DESC od;
	IDXGIOutput_GetDesc(output, &od);
	g_w = (int)(od.DesktopCoordinates.right - od.DesktopCoordinates.left);
	g_h = (int)(od.DesktopCoordinates.bottom - od.DesktopCoordinates.top);

	IDXGIOutput1 *output1 = NULL;
	hr = IDXGIOutput_QueryInterface(output, &IID_IDXGIOutput1, (void**)&output1);
	IDXGIOutput_Release(output);
	if (FAILED(hr) || !output1) { re_dxgi_release_all(); return -5; }

	hr = IDXGIOutput1_DuplicateOutput(output1, (IUnknown*)g_dev, &g_dup);
	IDXGIOutput1_Release(output1);
	if (FAILED(hr) || !g_dup) { re_dxgi_release_all(); return -6; }

	D3D11_TEXTURE2D_DESC td;
	memset(&td, 0, sizeof(td));
	td.Width = (UINT)g_w;
	td.Height = (UINT)g_h;
	td.MipLevels = 1;
	td.ArraySize = 1;
	td.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
	td.SampleDesc.Count = 1;
	td.Usage = D3D11_USAGE_STAGING;
	td.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
	hr = ID3D11Device_CreateTexture2D(g_dev, &td, NULL, &g_staging);
	if (FAILED(hr)) { re_dxgi_release_all(); return -7; }

	*w = g_w;
	*h = g_h;
	return 0;
}

int re_dxgi_capture(uint8_t **rgba, int *w, int *h, int *stride) {
	*rgba = NULL;
	if (!g_dup || !g_staging || !g_ctx) return -1;

	DXGI_OUTDUPL_FRAME_INFO fi;
	IDXGIResource *res = NULL;
	HRESULT hr = IDXGIOutputDuplication_AcquireNextFrame(g_dup, 50, &fi, &res);
	if (hr == DXGI_ERROR_WAIT_TIMEOUT) return -10;
	if (FAILED(hr)) return -2;

	ID3D11Texture2D *tex = NULL;
	hr = IDXGIResource_QueryInterface(res, &IID_ID3D11Texture2D, (void**)&tex);
	IDXGIResource_Release(res);
	if (FAILED(hr) || !tex) {
		IDXGIOutputDuplication_ReleaseFrame(g_dup);
		return -3;
	}

	ID3D11DeviceContext_CopyResource(g_ctx, (ID3D11Resource*)g_staging, (ID3D11Resource*)tex);
	ID3D11Texture2D_Release(tex);
	IDXGIOutputDuplication_ReleaseFrame(g_dup);

	D3D11_MAPPED_SUBRESOURCE map;
	hr = ID3D11DeviceContext_Map(g_ctx, (ID3D11Resource*)g_staging, 0, D3D11_MAP_READ, 0, &map);
	if (FAILED(hr)) return -4;

	size_t bpr = (size_t)g_w * 4;
	uint8_t *buf = (uint8_t*)malloc(bpr * (size_t)g_h);
	if (!buf) {
		ID3D11DeviceContext_Unmap(g_ctx, (ID3D11Resource*)g_staging, 0);
		return -5;
	}
	for (int y = 0; y < g_h; y++) {
		uint8_t *src = (uint8_t*)map.pData + y * map.RowPitch;
		uint8_t *dst = buf + y * bpr;
		for (int x = 0; x < g_w; x++) {
			dst[x*4+0] = src[x*4+2];
			dst[x*4+1] = src[x*4+1];
			dst[x*4+2] = src[x*4+0];
			dst[x*4+3] = 255;
		}
	}
	ID3D11DeviceContext_Unmap(g_ctx, (ID3D11Resource*)g_staging, 0);
	*rgba = buf;
	*w = g_w;
	*h = g_h;
	*stride = (int)bpr;
	return 0;
}

void re_dxgi_free(uint8_t *p) { free(p); }
void re_dxgi_close(void) { re_dxgi_release_all(); }
