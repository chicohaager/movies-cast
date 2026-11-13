// Media3's FFmpeg audio decoder (androidx/media 1.11.1, libraries/decoder_ffmpeg, Apache 2.0),
// built here because Google publishes it only as source. The FFmpeg libraries come from
// build-ffmpeg.sh (LGPL build); without them the module builds without native code and the
// player falls back to the device's own decoders.
plugins {
    alias(libs.plugins.android.library)
}

java { toolchain { languageVersion = JavaLanguageVersion.of(17) } }

android {
    namespace = "androidx.media3.decoder.ffmpeg"
    compileSdk = 37
    buildToolsVersion = "37.0.0"
    ndkVersion = "26.1.10909125"

    defaultConfig {
        minSdk = 23
        consumerProguardFiles("proguard-rules.txt")
    }

    if (file("src/main/jni/ffmpeg/android-libs").exists()) {
        externalNativeBuild {
            cmake {
                path = file("src/main/jni/CMakeLists.txt")
                version = "3.31.6"
            }
        }
    }
}

dependencies {
    api(libs.androidx.media3.decoder)
    implementation(libs.androidx.media3.exoplayer)
    implementation(libs.androidx.annotation)
    compileOnly(libs.checker.qual)
}
