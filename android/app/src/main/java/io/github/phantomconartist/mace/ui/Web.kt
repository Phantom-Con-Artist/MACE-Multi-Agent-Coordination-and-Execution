package io.github.phantomconartist.mace.ui

import android.content.Context
import android.net.Uri
import androidx.browser.customtabs.CustomTabColorSchemeParams
import androidx.browser.customtabs.CustomTabsIntent

// Hub monitoring, file browsing, and app launching live in the web app.
// The phone opens them in an in-app browser tab in MACE's colors.

fun openWeb(context: Context, url: String, toolbar: Int) {
    CustomTabsIntent.Builder()
        .setDefaultColorSchemeParams(CustomTabColorSchemeParams.Builder().setToolbarColor(toolbar).build())
        .setShowTitle(true)
        .build()
        .launchUrl(context, Uri.parse(url))
}
