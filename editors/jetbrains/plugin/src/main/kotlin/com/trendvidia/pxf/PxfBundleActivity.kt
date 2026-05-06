// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package com.trendvidia.pxf

import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.project.Project
import com.intellij.openapi.startup.ProjectActivity
import org.jetbrains.plugins.textmate.TextMateService
import org.jetbrains.plugins.textmate.configuration.TextMatePersistentBundle
import org.jetbrains.plugins.textmate.configuration.TextMateUserBundlesSettings
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths
import java.nio.file.StandardCopyOption

private val BUNDLE_RESOURCES = listOf(
    "pxf.tmbundle/info.plist",
    "pxf.tmbundle/Syntaxes/PXF.tmLanguage",
    "pxf.tmbundle/Syntaxes/pxf.tmLanguage.json",
    "pxf.tmbundle/Preferences/Comments.tmPreferences",
)

class PxfBundleActivity : ProjectActivity {
    override suspend fun execute(project: Project) {
        registerOnce()
    }

    companion object {
        @Volatile
        private var attempted = false

        @Synchronized
        fun registerOnce() {
            if (attempted) return
            attempted = true

            val bundleDir = try {
                extractBundle()
            } catch (e: Throwable) {
                thisLogger().warn("PXF: failed to extract embedded TextMate bundle", e)
                return
            }

            try {
                val settings = TextMateUserBundlesSettings.getInstance() ?: run {
                    thisLogger().warn("PXF: TextMateUserBundlesSettings not available; is the TextMate plugin enabled?")
                    return
                }

                val bundleKey = bundleDir.toString()
                val current = settings.bundles
                val existing = current[bundleKey]
                if (existing != null && existing.enabled) {
                    thisLogger().info("PXF: TextMate bundle already registered at $bundleDir")
                    return
                }

                val updated = current.toMutableMap()
                updated[bundleKey] = TextMatePersistentBundle("PXF", true)
                settings.setBundlesConfig(updated)
                TextMateService.getInstance().reloadEnabledBundles()
                thisLogger().info("PXF: registered TextMate bundle at $bundleDir")
            } catch (e: Throwable) {
                thisLogger().warn(
                    "PXF: failed to auto-register TextMate bundle. " +
                        "You can add it manually: Settings → Editor → TextMate Bundles → + → $bundleDir",
                    e,
                )
            }
        }

        private fun extractBundle(): Path {
            val home = Paths.get(System.getProperty("user.home"), ".cache", "pxf-jetbrains")
            val bundleDir = home.resolve("pxf.tmbundle")
            Files.createDirectories(bundleDir)

            val cl = PxfBundleActivity::class.java.classLoader
            for (resource in BUNDLE_RESOURCES) {
                val dest = home.resolve(resource)
                Files.createDirectories(dest.parent)
                val stream = cl.getResourceAsStream(resource)
                    ?: error("PXF: resource missing from plugin JAR: $resource")
                stream.use { Files.copy(it, dest, StandardCopyOption.REPLACE_EXISTING) }
            }
            return bundleDir
        }
    }
}
