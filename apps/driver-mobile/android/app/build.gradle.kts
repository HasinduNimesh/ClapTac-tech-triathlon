import java.util.Properties

plugins {
    id("com.android.application")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

// Release signing comes from android/key.properties, which holds the path to a private keystore and its
// passwords. That file and the keystore stay out of Git (see android/.gitignore). Without it a release
// build fails on purpose rather than falling back to the debug key; see the README.
val keystoreProperties = Properties().apply {
    val file = rootProject.file("key.properties")
    if (file.exists()) file.inputStream().use { load(it) }
}
val hasReleaseKeystore = keystoreProperties.getProperty("storeFile") != null
// Only for local testing of a release build: sign with the debug key when WAYPOINT_ALLOW_DEBUG_SIGNING=1.
val allowDebugSigning = System.getenv("WAYPOINT_ALLOW_DEBUG_SIGNING") == "1"

android {
    namespace = "dev.claptac.waypoint_driver"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        // TODO: Specify your own unique Application ID (https://developer.android.com/studio/build/application-id.html).
        applicationId = "dev.claptac.waypoint_driver"
        // You can update the following values to match your application needs.
        // For more information, see: https://flutter.dev/to/review-gradle-config.
        // flutter_secure_storage needs API 23 or newer.
        minSdk = maxOf(flutter.minSdkVersion, 23)
        targetSdk = flutter.targetSdkVersion
        // Uses the version code from pubspec.yaml. When using split APKs, 1000 * ABI_VERSION
        // is added automatically by Flutter. (https://developer.android.com/studio/build/configure-apk-splits#configure-APK-versions)
        // You can force using the value of versionCode by specifying the `-P force-version-code-ignoring-abi=true`
        // flag during build.
        versionCode = flutter.versionCode
        versionName = flutter.versionName
        // The sign-in redirect (dev.claptac.waypointdriver:/oauth2redirect) comes back to this app.
        // Keep it in step with OIDC_REDIRECT_URI.
        manifestPlaceholders["appAuthRedirectScheme"] = "dev.claptac.waypointdriver"
    }

    signingConfigs {
        create("release") {
            if (hasReleaseKeystore) {
                keyAlias = keystoreProperties.getProperty("keyAlias")
                keyPassword = keystoreProperties.getProperty("keyPassword")
                storeFile = rootProject.file(keystoreProperties.getProperty("storeFile"))
                storePassword = keystoreProperties.getProperty("storePassword")
            }
        }
    }

    buildTypes {
        release {
            signingConfig = if (hasReleaseKeystore) signingConfigs.getByName("release") else signingConfigs.getByName("debug")
        }
    }
}

// Refuse to build a release that would be signed with the debug key, unless that was asked for explicitly.
gradle.taskGraph.whenReady {
    val buildsRelease = allTasks.any { it.project == project && it.name.contains("Release", ignoreCase = true) }
    if (buildsRelease && !hasReleaseKeystore && !allowDebugSigning) {
        throw GradleException(
            "No release keystore configured. Create android/key.properties (storeFile, storePassword, keyAlias, " +
                "keyPassword) pointing at a private keystore, or set WAYPOINT_ALLOW_DEBUG_SIGNING=1 to build a " +
                "release for local testing only (debug-signed, not for distribution)."
        )
    }
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}

flutter {
    source = "../.."
}
