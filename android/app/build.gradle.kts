import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
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
    buildToolsVersion = "37.0.0"

    defaultConfig {
        applicationId = "io.github.chicohaager.moviescast"
        minSdk = 23
        targetSdk = 37
        versionCode = 100
        versionName = "0.1.0"
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
    implementation(libs.androidx.media3.session)
    implementation(libs.androidx.media3.ui)

    implementation(libs.coil.compose)
    implementation(libs.coil.network.okhttp)
    implementation(libs.kotlinx.coroutines.android)

    testImplementation(libs.junit)
}
