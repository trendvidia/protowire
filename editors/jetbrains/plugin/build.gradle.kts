// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

plugins {
    id("org.jetbrains.kotlin.jvm") version "2.4.10"
    id("org.jetbrains.intellij.platform") version "2.18.1"
}

group = "com.trendvidia.pxf"
version = "1.0.0"

repositories {
    mavenCentral()
    intellijPlatform {
        defaultRepositories()
    }
}

dependencies {
    intellijPlatform {
        intellijIdeaCommunity("2024.3")
        bundledPlugin("org.jetbrains.plugins.textmate")
    }

    // PXF parser bundled from protowire-java's :pxf module.
    //
    // TODO(packaging): switch to a Maven Central coordinate
    //   `com.trendvidia.protowire:pxf:<version>`
    // once protowire-java is published. Until then we ship a vendored copy
    // of the parser jar at libs/protowire-pxf.jar; refresh it with
    //   scripts/refresh_jetbrains_parser_jar.sh
    // The jar contains protobuf-java-using classes (Encoder, Pxf, …) that
    // are NEVER loaded by the plugin — we only call Parser.parse(String),
    // whose transitive dependencies are pure JDK.
    implementation(files("libs/protowire-pxf.jar"))
}

kotlin {
    jvmToolchain(21)
}

intellijPlatform {
    pluginConfiguration {
        ideaVersion {
            sinceBuild = "243"
            untilBuild = provider { null }
        }
    }
    buildSearchableOptions = false
    instrumentCode = false
}

tasks {
    buildPlugin {
        archiveBaseName.set("pxf-jetbrains")
    }
}
