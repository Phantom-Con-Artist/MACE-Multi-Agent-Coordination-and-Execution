import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

// One version for the whole repository, from its VERSION file.
val maceVersion = rootProject.file("../VERSION").readText().trim()
val (major, minor, patch) = maceVersion.substringBefore("-").split(".").map { it.toInt() }

// Release signing comes from android/keystore.properties (never committed)
// or from the environment in CI. Without it, release builds are unsigned.
val signing = Properties().apply {
    val file = rootProject.file("keystore.properties")
    if (file.exists()) file.inputStream().use { load(it) }
    System.getenv("MACE_KEYSTORE")?.let { setProperty("storeFile", it) }
    System.getenv("MACE_KEYSTORE_PASSWORD")?.let { setProperty("storePassword", it); setProperty("keyPassword", it) }
    System.getenv("MACE_KEY_ALIAS")?.let { setProperty("keyAlias", it) }
}

android {
    namespace = "io.github.phantomconartist.mace"
    compileSdk = 36

    defaultConfig {
        applicationId = "io.github.phantomconartist.mace"
        minSdk = 29
        targetSdk = 36
        versionCode = major * 10000 + minor * 100 + patch
        versionName = maceVersion
        // The client API this build speaks; see docs/API.md.
        buildConfigField("int", "MACE_API", "1")
    }

    signingConfigs {
        if (signing.getProperty("storeFile") != null) {
            create("release") {
                storeFile = file(signing.getProperty("storeFile"))
                storePassword = signing.getProperty("storePassword")
                keyAlias = signing.getProperty("keyAlias")
                keyPassword = signing.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.findByName("release")
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

base.archivesName.set("mace-android-$maceVersion")

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    debugImplementation(libs.compose.ui.tooling)
    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
}
