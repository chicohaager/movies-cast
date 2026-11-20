import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.aboutlibraries)
}

java { toolchain { languageVersion = JavaLanguageVersion.of(17) } }

// One entry per app/src/main/res/values-*, in the spelling of the folder. The build keeps these
// translations in the APK and the app offers them in its language list;
// deploy/tools/check-android.sh compares the APK with the folders.
val languages = listOf("en", "cs", "de", "es", "fr", "hu", "it", "ja", "ko", "nl", "pl", "pt-rBR", "pt-rPT", "ru", "sv", "tr", "zh-rCN", "zh-rTW")

// The release key is not in the repository. A properties file names it (storeFile, storePassword,
// keyAlias, keyPassword): the one MOVIESCAST_KEYSTORE_PROPERTIES points to, else keystore.properties
// next to this build, else android-keystore.properties in ~/.local/share/movies-cast, where the key
// of the Android TV app (deploy/androidtv) lies as well. Without any, the release build is unsigned.
val keystoreFile = System.getenv("MOVIESCAST_KEYSTORE_PROPERTIES")?.let(::File)
    ?: rootProject.file("keystore.properties").takeIf { it.isFile }
    ?: File(System.getProperty("user.home"), ".local/share/movies-cast/android-keystore.properties")
val keystore = Properties().apply { if (keystoreFile.isFile) keystoreFile.inputStream().use { load(it) } }

android {
    namespace = "io.github.chicohaager.moviescast"
    compileSdk = 37
    // The NDK of the ffmpeg module: AGP strips the native libraries with the NDK named here, and without one
    // it looks for its own default (28.2), finds none and packs them unstripped, with debug info and the
    // build machine's paths (0.2.0 and the first 0.3.0 build, 2026-11-20). check-android.sh fails on that.
    ndkVersion = "26.1.10909125"
    buildToolsVersion = "37.0.0"

    defaultConfig {
        applicationId = "io.github.chicohaager.moviescast"
        minSdk = 24
        targetSdk = 37
        versionCode = 300
        versionName = "0.3.0"
        // The tests on a device (src/androidTest) run the app with a store of its own, see TestRunner.
        testInstrumentationRunner = "io.github.chicohaager.moviescast.TestRunner"
        buildConfigField("String[]", "LANGUAGES", languages.joinToString(prefix = "{", postfix = "}") { "\"$it\"" })
    }

    signingConfigs {
        if (keystoreFile.isFile) {
            create("release") {
                storeFile = keystoreFile.resolveSibling(keystore.getProperty("storeFile"))
                storePassword = keystore.getProperty("storePassword")
                keyAlias = keystore.getProperty("keyAlias")
                keyPassword = keystore.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        named("debug") { applicationIdSuffix = ".debug" }
        named("release") {
            isMinifyEnabled = true
            isShrinkResources = true
            signingConfig = signingConfigs.findByName("release")
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    buildFeatures {
        buildConfig = true
        compose = true
    }

    androidResources {
        localeFilters += languages
        generateLocaleConfig = true
    }

    dependenciesInfo {
        includeInApk = false
        includeInBundle = false
    }
}

// The licences page of the app: every library of the build, read by the plugin into
// res/raw/aboutlibraries.json (as Jellyfin's Android TV app does). FFmpeg and the copied Media3
// decoder are no Gradle dependencies; ../config names them and carries the LGPL's text as FFmpeg
// ships it (COPYING.LGPLv2.1), so the page does not depend on what the plugin can fetch.
aboutLibraries {
    collect { configPath = file("../config") }
}

// The privacy policy is a page of the app (a TV has no browser): the repository's PRIVACY.md goes
// into the APK as assets/PRIVACY.md, so the app and the repository cannot say different things.
abstract class PrivacyAsset : DefaultTask() {
    @get:InputFile
    abstract val source: RegularFileProperty

    @get:OutputDirectory
    abstract val outputDir: DirectoryProperty

    @TaskAction
    fun copy() {
        source.get().asFile.copyTo(outputDir.get().asFile.resolve("PRIVACY.md"), overwrite = true)
    }
}
val privacyAsset = tasks.register<PrivacyAsset>("privacyAsset") { source.set(rootProject.file("../PRIVACY.md")) }
androidComponents {
    onVariants { variant -> variant.sources.assets?.addGeneratedSourceDirectory(privacyAsset, PrivacyAsset::outputDir) }
}

dependencies {
    implementation(project(":core"))
    implementation(project(":ffmpeg"))

    implementation(libs.androidx.core)
    implementation(libs.androidx.appcompat)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.compose.foundation)
    implementation(libs.androidx.compose.runtime)
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.tooling.preview)
    debugImplementation(libs.androidx.compose.ui.tooling)
    implementation(libs.androidx.compose.material3)

    implementation(libs.androidx.media3.exoplayer)
    implementation(libs.androidx.media3.exoplayer.hls)
    implementation(libs.androidx.media3.datasource.okhttp)
    // reads a file's tracks without playing it, for the film page's facts (FileProbe.kt)
    implementation(libs.androidx.media3.inspector)
    implementation(libs.androidx.media3.session)
    implementation(libs.androidx.media3.ui)

    implementation(libs.coil.compose)
    implementation(libs.coil.network.okhttp)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.aboutlibraries.core)

    testImplementation(libs.junit)

    androidTestImplementation(libs.junit)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    // Compose's test library brings Espresso 3.5.0, which fails on Android 17 with
    // "NoSuchMethodException: InputManager.getInstance" (Pixel 7, measured 2026-11-13).
    androidTestImplementation(libs.androidx.test.espresso.core)
    androidTestImplementation(libs.okhttp.mockwebserver)
    androidTestImplementation(libs.okhttp.tls)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}
