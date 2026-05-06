// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package com.trendvidia.pxf.actions

import com.intellij.ide.actions.CreateFileFromTemplateAction
import com.intellij.ide.actions.CreateFileFromTemplateDialog
import com.intellij.openapi.project.Project
import com.intellij.psi.PsiDirectory

class NewPxfFileAction : CreateFileFromTemplateAction(
    "PXF File",
    "Create a new PXF (Proto eXpressive Format) file",
    null,
) {
    override fun buildDialog(
        project: Project,
        directory: PsiDirectory,
        builder: CreateFileFromTemplateDialog.Builder,
    ) {
        builder
            .setTitle("New PXF File")
            .addKind("PXF file", null, "PXF File")
    }

    override fun getActionName(directory: PsiDirectory, newName: String, templateName: String): String =
        "Create PXF file: $newName"
}
