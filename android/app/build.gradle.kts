import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

/**
 * Upload-key material, from a local `keystore.properties` or the environment.
 *
 * Nothing about the key is committed: the file is git-ignored and CI hands the
 * values in from Vault. When none is available the release build is simply
 * unsigned, so `assembleRelease` still works for anyone checking that R8 and
 * the proguard rules hold.
 */
val keystoreProperties = Properties().apply {
    val file = rootProject.file("keystore.properties")
    if (file.exists()) file.inputStream().use { load(it) }
}

fun signingValue(key: String, env: String): String? =
    keystoreProperties.getProperty(key) ?: System.getenv(env)

val uploadStorePath = signingValue("storeFile", "RP_ANDROID_KEYSTORE")
val uploadStorePassword = signingValue("storePassword", "RP_ANDROID_KEYSTORE_PASSWORD")
val uploadKeyAlias = signingValue("keyAlias", "RP_ANDROID_KEY_ALIAS")
val uploadKeyPassword = signingValue("keyPassword", "RP_ANDROID_KEY_PASSWORD")
val hasUploadKey = listOf(
    uploadStorePath, uploadStorePassword, uploadKeyAlias, uploadKeyPassword,
).all { !it.isNullOrEmpty() } && file(uploadStorePath!!).exists()

android {
    namespace = "org.fictionally.ringpromoter"
    compileSdk = 37

    defaultConfig {
        // Matches the iOS bundle identifier. Permanent once published to Play:
        // a package name can never be changed, reused or freed, not even after
        // the app is deleted.
        applicationId = "org.fictionally.ringpromoter"
        minSdk = 26
        targetSdk = 36
        // A fresh Play listing, not a continuation of iOS 1.10.
        //
        // CI overrides both from the command line: the version code comes from
        // what Play has already seen, so a rebuild of the same commit never
        // collides with an uploaded bundle.
        versionCode = (project.findProperty("versionCode") as String?)?.toInt() ?: 1
        versionName = (project.findProperty("versionName") as String?) ?: "1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    signingConfigs {
        if (hasUploadKey) {
            create("upload") {
                storeFile = file(uploadStorePath!!)
                storePassword = uploadStorePassword
                keyAlias = uploadKeyAlias
                keyPassword = uploadKeyPassword
            }
        }
    }

    buildTypes {
        release {
            // Unsigned when no upload key is configured, rather than failing:
            // a contributor checking that R8 holds needs no credentials.
            signingConfig = if (hasUploadKey) signingConfigs.getByName("upload") else null
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
    }

    packaging {
        resources.excludes += "/META-INF/{AL2.0,LGPL2.1}"
    }

    testOptions {
        unitTests.isIncludeAndroidResources = true
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.navigation.compose)

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.material.icons.extended)

    implementation(libs.androidx.biometric)
    implementation(libs.androidx.glance.appwidget)
    implementation(libs.androidx.glance.material3)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)

    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)

    androidTestImplementation(libs.androidx.test.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
}
