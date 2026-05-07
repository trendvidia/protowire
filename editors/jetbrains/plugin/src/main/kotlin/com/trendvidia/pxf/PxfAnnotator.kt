// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package com.trendvidia.pxf

import com.intellij.lang.annotation.AnnotationHolder
import com.intellij.lang.annotation.ExternalAnnotator
import com.intellij.lang.annotation.HighlightSeverity
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.util.TextRange
import com.intellij.psi.PsiFile
import org.protowire.pxf.Parser
import org.protowire.pxf.PxfException

/**
 * Phase-1 PXF validator: runs the protowire-java parser on every .pxf file
 * and surfaces parse errors as inline error annotations.
 *
 * Schema-aware validation (field/type checking against a descriptor set) is
 * intentionally NOT done here — that's phase 2 and needs a per-project
 * descriptor-set setting. See README.md → "PXF validation" for the roadmap.
 */
class PxfAnnotator : ExternalAnnotator<PxfAnnotator.Input, List<PxfAnnotator.Issue>>() {

    data class Input(val text: String, val fileName: String)
    data class Issue(val line: Int, val column: Int, val message: String)

    override fun collectInformation(file: PsiFile, editor: Editor, hasErrors: Boolean): Input? {
        if (!file.name.endsWith(".pxf", ignoreCase = true)) return null
        return Input(editor.document.text, file.name)
    }

    override fun collectInformation(file: PsiFile): Input? {
        if (!file.name.endsWith(".pxf", ignoreCase = true)) return null
        return Input(file.text, file.name)
    }

    override fun doAnnotate(input: Input?): List<Issue> {
        if (input == null) return emptyList()
        return try {
            Parser.parse(input.text)
            emptyList()
        } catch (e: PxfException) {
            val pos = e.position()
            // PxfException prefixes its message with "<line>:<column>: " — strip
            // it because IntelliJ already renders position context next to the
            // squiggle, and the duplicate just clutters the hover tooltip.
            val raw = e.message ?: "PXF parse error"
            val cleaned = raw.removePrefix("${pos.line()}:${pos.column()}: ")
            listOf(Issue(pos.line(), pos.column(), cleaned))
        } catch (e: Throwable) {
            // Defensive: any unexpected parser failure becomes a single
            // file-level annotation rather than crashing the analyzer thread.
            listOf(Issue(1, 1, "PXF parse error: ${e.message ?: e.javaClass.simpleName}"))
        }
    }

    override fun apply(file: PsiFile, issues: List<Issue>?, holder: AnnotationHolder) {
        if (issues.isNullOrEmpty()) return
        val text = file.text
        for (issue in issues) {
            val range = lineColumnToRange(text, issue.line, issue.column)
            holder.newAnnotation(HighlightSeverity.ERROR, issue.message)
                .range(range)
                .create()
        }
    }

    /**
     * Map a 1-based (line, column) reported by the PXF parser to an
     * IntelliJ TextRange. The range covers a single character; if the
     * coordinates fall past EOF we anchor at the last valid offset so the
     * annotation is still rendered.
     */
    private fun lineColumnToRange(text: String, line: Int, column: Int): TextRange {
        val lineStart = nthLineStart(text, line)
        val rawOffset = lineStart + (column - 1).coerceAtLeast(0)
        val end = text.length
        val start = rawOffset.coerceIn(0, end)
        val finish = (start + 1).coerceAtMost(end)
        return if (start == finish && start > 0) TextRange(start - 1, start) else TextRange(start, finish)
    }

    private fun nthLineStart(text: String, line: Int): Int {
        if (line <= 1) return 0
        var seen = 1
        var i = 0
        while (i < text.length) {
            if (text[i] == '\n') {
                seen++
                if (seen == line) return i + 1
            }
            i++
        }
        return text.length
    }
}
