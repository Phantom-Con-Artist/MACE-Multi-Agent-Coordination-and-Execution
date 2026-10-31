package io.github.phantomconartist.mace.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext

// MACE for Android is a native Material 3 app: colors come from the phone's
// wallpaper (Android 12 and later), type from the system font. Older phones
// get a scheme seeded from the MACE red.

private val FallbackLight = lightColorScheme(primary = Color(0xFF9C413A), secondary = Color(0xFF775653), tertiary = Color(0xFF715B2E))
private val FallbackDark = darkColorScheme(primary = Color(0xFFFFB4AA), secondary = Color(0xFFE7BDB8), tertiary = Color(0xFFE0C38C))

@Composable
fun MaceTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val scheme = when {
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
            if (dark) dynamicDarkColorScheme(LocalContext.current) else dynamicLightColorScheme(LocalContext.current)
        dark -> FallbackDark
        else -> FallbackLight
    }
    MaterialTheme(colorScheme = scheme, content = content)
}
