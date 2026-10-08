/* akvirtualcamera, virtual camera for Mac and Windows.
 * Copyright (C) 2026  Gonzalo Exequiel Pedone
 *
 * akvirtualcamera is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * akvirtualcamera is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with akvirtualcamera. If not, see <http://www.gnu.org/licenses/>.
 *
 * Web-Site: http://webcamoid.github.io/
 */

#import "extensionprovidersource.h"
#import "extensiondevicesource.h"
#import "extensionstreamsource.h"
#include <string>
#include "PlatformUtils/src/preferences.h"
#include "VCamUtils/src/fraction.h"
#include "VCamUtils/src/ipcbridge.h"
#include "VCamUtils/src/logger.h"
#include "VCamUtils/src/videoformat.h"

@interface ExtensionProviderSource () {
    AkVCam::IpcBridgePtr m_ipcBridge;

    // Maps deviceId to ExtensionDeviceSource for all currently registered devices.
    NSMutableDictionary<NSString *, ExtensionDeviceSource *> *m_deviceSources;
}
@end

static void devicesChangedCallback(void *userData,
                                   const std::vector<std::string> &devices);
static void frameReadyCallback(void *userData,
                               const std::string &deviceId,
                               const AkVCam::VideoFrame &frame,
                               bool isActive);
static void pictureChangedCallback(void *userData,
                                   const std::string &picture);
static void controlsChangedCallback(void *userData,
                                    const std::string &deviceId,
                                    const std::map<std::string, int> &controls);

@implementation ExtensionProviderSource

/* Creates the IpcBridge, subscribes to its four callbacks, starts
 * notifications, and registers one ExtensionDeviceSource for each device
 * the daemon already knows about.
 */
/* Mirror Apple's / VCamCX pattern: create the CMIOExtensionProvider in init
 * before addDevice. Creating devices first (provider=nil) left the provider
 * empty and browsers only saw the built-in camera.
 */
- (instancetype) init
{
    AkLogFunction();

    if (self = [super init]) {
        m_deviceSources = [NSMutableDictionary dictionary];
        m_ipcBridge = std::make_shared<AkVCam::IpcBridge>();

        m_ipcBridge->connectDevicesChanged((__bridge void *) self,
                                           devicesChangedCallback);
        m_ipcBridge->connectFrameReady((__bridge void *) self,
                                       frameReadyCallback);
        m_ipcBridge->connectPictureChanged((__bridge void *) self,
                                           pictureChangedCallback);
        m_ipcBridge->connectControlsChanged((__bridge void *) self,
                                            controlsChangedCallback);

        _provider =
            [CMIOExtensionProvider
             providerWithSource: self
             clientQueue: dispatch_get_main_queue()];

        auto devices = m_ipcBridge->devices();

        for (auto &deviceId: devices) {
            auto description = m_ipcBridge->description(deviceId);
            auto formats     = m_ipcBridge->formats(deviceId);
            // Drop RGB24 and prefer RGB32 — RGB24-first breaks browser color/listing.
            std::vector<AkVCam::VideoFormat> filtered;

            for (auto &fmt: formats)
                if (fmt.format() != AkVCam::PixelFormat_rgb24)
                    filtered.push_back(fmt);

            formats.swap(filtered);

            if (formats.empty()) {
                formats.push_back(
                    AkVCam::VideoFormat(AkVCam::PixelFormat_argb, 1280, 720,
                                        AkVCam::Fraction(30, 1)));
            }
            [self createDeviceWithId: deviceId
                         description: description
                             formats: formats];
        }

        if (devices.empty()) {
            AkPrintErr("No devices from prefs — registering default KoKoPhoneCam0");
            std::vector<AkVCam::VideoFormat> formats = {
                AkVCam::VideoFormat(AkVCam::PixelFormat_argb, 1280, 720,
                                    AkVCam::Fraction(30, 1))
            };
            [self createDeviceWithId: "KoKoPhoneCam0"
                         description: "KoKo Phone Camera"
                             formats: formats];
        }
    }

    return self;
}

- (void) dealloc
{
    AkLogFunction();
    m_ipcBridge->stopNotifications();
}

/* Allocates an ExtensionDeviceSource, wires it to a new CMIOExtensionStream
 * and CMIOExtensionDevice, then registers the device with the provider.
 * Does nothing if a device with that ID already exists.
 */
- (void) createDeviceWithId: (const std::string &) deviceId
                description: (const std::string &) description
                    formats: (const std::vector<AkVCam::VideoFormat> &) formats
{
    AkLogFunction();

    NSString *deviceIdStr = @(deviceId.c_str());

    if (m_deviceSources[deviceIdStr]) {
        AkLogWarning("Device already registered: %s", deviceId.c_str());
        return;
    }

    ExtensionDeviceSource *deviceSource =
        [[ExtensionDeviceSource alloc] initWithDeviceId: deviceId
                                            description: description
                                                formats: formats];
    [deviceSource setBridge: m_ipcBridge];

    /* Prefer a real UUID device id; otherwise derive a stable UUID from the
     * legacy id string. A random UUID here made the camera disappear from
     * browsers after each Extension replace (Chrome caches by deviceId).
     */
    NSUUID *deviceUUID = [[NSUUID alloc] initWithUUIDString: deviceIdStr];

    if (!deviceUUID) {
        std::string seed = std::string("akvcam.device.") + deviceId;
        uuid_t hashed {};
        uint64_t h1 = 14695981039346656037ull;
        uint64_t h2 = 1099511628211ull;

        for (unsigned char c: seed) {
            h1 ^= c;
            h1 *= 1099511628211ull;
            h2 ^= static_cast<unsigned char>(c * 31u);
            h2 *= 14695981039346656037ull;
        }

        for (int i = 0; i < 8; ++i) {
            hashed[i] = static_cast<uint8_t>((h1 >> (8 * (7 - i))) & 0xff);
            hashed[8 + i] = static_cast<uint8_t>((h2 >> (8 * (7 - i))) & 0xff);
        }

        hashed[6] = (hashed[6] & 0x0f) | 0x50; // version 5
        hashed[8] = (hashed[8] & 0x3f) | 0x80; // RFC 4122 variant
        deviceUUID = [[NSUUID alloc] initWithUUIDBytes: hashed];
    }

    uuid_t uuidBytes;
    [deviceUUID getUUIDBytes: uuidBytes];
    uuidBytes[0] ^= 0x01;
    NSUUID *streamUUID = [[NSUUID alloc] initWithUUIDBytes: uuidBytes];

    CMIOExtensionStream *stream =
        [[CMIOExtensionStream alloc]
         initWithLocalizedName: @"Video"
         streamID: streamUUID
         direction: CMIOExtensionStreamDirectionSource
         clockType: CMIOExtensionStreamClockTypeHostTime
         source: deviceSource.streamSource];

    deviceSource.streamSource.stream = stream;

    CMIOExtensionDevice *device =
        [[CMIOExtensionDevice alloc]
         initWithLocalizedName: @(description.c_str())
         deviceID: deviceUUID
         legacyDeviceID: deviceIdStr
         source: deviceSource];

    deviceSource.device = device;

    NSError *error = nil;
    [device addStream: stream error: &error];

    if (error) {
        AkPrintErr("Error adding stream to device %s: %s",
                   deviceId.c_str(),
                   [[error description] UTF8String]);
        return;
    }

    m_deviceSources[deviceIdStr] = deviceSource;

    if (self.provider) {
        error = nil;
        [self.provider addDevice: device error: &error];

        if (error)
            AkPrintErr("Error registering device %s with provider: %s",
                       deviceId.c_str(),
                       [[error description] UTF8String]);
    }
}

/* Stops any active stream, removes the device from the provider, and
 * discards the ExtensionDeviceSource for the given device ID.
 */
- (void) destroyDeviceWithId: (const std::string &) deviceId
{
    AkLogFunction();

    NSString *deviceIdStr = @(deviceId.c_str());
    ExtensionDeviceSource *deviceSource = m_deviceSources[deviceIdStr];

    if (!deviceSource) {
        AkLogWarning("Device not found: %s", deviceId.c_str());
        return;
    }

    [deviceSource stopStreaming];

    if (self.provider && deviceSource.device) {
        NSError *error = nil;
        [self.provider removeDevice: deviceSource.device error: &error];

        if (error)
            AkPrintErr("Error removing device %s from provider: %s",
                       deviceId.c_str(),
                       [[error description] UTF8String]);
    }

    [m_deviceSources removeObjectForKey: deviceIdStr];
}

/* Destroys all existing devices and recreates them from the updated device
 * list reported by the daemon.
 */
static void devicesChangedCallback(void *userData,
                                   const std::vector<std::string> &devices)
{
    AkLogFunction();

    auto self = (__bridge ExtensionProviderSource *) userData;

    // Collect IDs currently registered.
    NSArray<NSString *> *existing =
        [self->m_deviceSources.allKeys copy];

    for (NSString *deviceIdStr in existing) {
        std::string deviceId = [deviceIdStr UTF8String];
        [self destroyDeviceWithId: deviceId];
    }

    for (auto &deviceId: self->m_ipcBridge->devices()) {
        auto description = self->m_ipcBridge->description(deviceId);
        auto formats     = self->m_ipcBridge->formats(deviceId);
        std::vector<AkVCam::VideoFormat> filtered;

        for (auto &fmt: formats)
            if (fmt.format() != AkVCam::PixelFormat_rgb24)
                filtered.push_back(fmt);

        formats.swap(filtered);

        if (formats.empty()) {
            formats.push_back(
                AkVCam::VideoFormat(AkVCam::PixelFormat_argb, 1280, 720,
                                    AkVCam::Fraction(30, 1)));
        }

        [self createDeviceWithId: deviceId
                     description: description
                         formats: formats];
    }
}

/* Forwards an incoming frame to the ExtensionDeviceSource identified by
 * deviceId.
 */
static void frameReadyCallback(void *userData,
                               const std::string &deviceId,
                               const AkVCam::VideoFrame &frame,
                               bool isActive)
{
    AkLogFunction();

    auto self = (__bridge ExtensionProviderSource *) userData;
    NSString *deviceIdStr = @(deviceId.c_str());
    ExtensionDeviceSource *deviceSource = self->m_deviceSources[deviceIdStr];

    if (deviceSource)
        [deviceSource frameReady: frame isActive: isActive];
}

// Forwards the new picture path to every registered device source.
static void pictureChangedCallback(void *userData,
                                   const std::string &picture)
{
    AkLogFunction();

    auto self = (__bridge ExtensionProviderSource *) userData;

    for (ExtensionDeviceSource *deviceSource in self->m_deviceSources.allValues)
        [deviceSource setPicture: picture];
}

// Forwards updated controls to the device source identified by deviceId.
static void controlsChangedCallback(void *userData,
                                    const std::string &deviceId,
                                    const std::map<std::string, int> &controls)
{
    AkLogFunction();

    auto self = (__bridge ExtensionProviderSource *) userData;
    NSString *deviceIdStr = @(deviceId.c_str());
    ExtensionDeviceSource *deviceSource = self->m_deviceSources[deviceIdStr];

    if (deviceSource)
        [deviceSource setControls: controls];
}

// Declares the provider-level properties exposed to the CoreMediaIO framework.
- (NSSet<CMIOExtensionProperty> *) availableProperties
{
    AkLogFunction();

    return [NSSet setWithObject: CMIOExtensionPropertyProviderManufacturer];
}

// Returns the current values for the requested provider properties.
- (nullable CMIOExtensionProviderProperties *) providerPropertiesForProperties: (NSSet<CMIOExtensionProperty> *) properties
                                               error: (NSError **) outError
{
    AkLogFunction();

    CMIOExtensionProviderProperties *providerProperties =
        [CMIOExtensionProviderProperties providerPropertiesWithDictionary: @{}];

    if ([properties containsObject: CMIOExtensionPropertyProviderManufacturer])
        providerProperties.manufacturer = @COMMONS_APPNAME;

    return providerProperties;
}

- (BOOL) setProviderProperties: (CMIOExtensionProviderProperties *) providerProperties
                         error: (NSError **) outError
{
    AkLogFunction();

    return YES;
}

/* Called when a client application connects to the provider.
 * Returns YES to accept all connections.
 */
- (BOOL) connectClient: (CMIOExtensionClient *) client
                 error: (NSError * _Nullable *) outError
{
    AkLogFunction();

    return YES;
}

// Called when a client application disconnects from the provider.
- (void) disconnectClient: (CMIOExtensionClient *) client
{
    AkLogFunction();

    const char *clientDesc = client? [[client description] UTF8String]: "nil";
    AkPrintOut("Client disconnected: %s\n", clientDesc);
}

@end
