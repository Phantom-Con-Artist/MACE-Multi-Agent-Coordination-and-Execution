package io.github.phantomconartist.mace.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.R

// The palette is web/src/theme.css, light and dark, so the app and the
// desktop read as one product. No pure black or white surfaces.

@Immutable
data class MaceColors(
    val chrome: Color,
    val chromeInk: Color,
    val chromeMuted: Color,
    val surface: Color,
    val soft: Color,
    val line: Color,
    val text: Color,
    val muted: Color,
    val accent: Color,
    val accentInk: Color,
    val accentSoft: Color,
    val bubble: Color,
    val bubbleInk: Color,
    val incoming: Color,
    val chat: Color,
    val errorBg: Color,
    val errorText: Color,
)

private val Light = MaceColors(
    chrome = Color(0xFF161112), chromeInk = Color(0xFFF6F1EC), chromeMuted = Color(0xFFA39A96),
    surface = Color(0xFFFFFDFA), soft = Color(0xFFF3EEE8), line = Color(0xFFE7E0D7),
    text = Color(0xFF171213), muted = Color(0xFF6B6160),
    accent = Color(0xFFA20000), accentInk = Color(0xFFFFF8F4), accentSoft = Color(0xFFF6E2DD),
    bubble = Color(0xFFF6E2DD), bubbleInk = Color(0xFF2B0D0B), incoming = Color(0xFFFFFDFA), chat = Color(0xFFF1EBE3),
    errorBg = Color(0xFFFBE7E2), errorText = Color(0xFF87160D),
)

private val Dark = MaceColors(
    chrome = Color(0xFF050404), chromeInk = Color(0xFFF3ECE8), chromeMuted = Color(0xFF8F8481),
    surface = Color(0xFF141011), soft = Color(0xFF1F1A1A), line = Color(0xFF2F2828),
    text = Color(0xFFF4EEEA), muted = Color(0xFFA99D99),
    accent = Color(0xFFB80D07), accentInk = Color(0xFFFFF6F2), accentSoft = Color(0xFF3A1714),
    bubble = Color(0xFF4A1511), bubbleInk = Color(0xFFFDEEEA), incoming = Color(0xFF1D1818), chat = Color(0xFF0D0A0A),
    errorBg = Color(0xFF3C1A17), errorText = Color(0xFFFFB4A8),
)

val LocalMaceColors = staticCompositionLocalOf { Light }

/** Archivo, self-hosted (res/font/archivo.ttf), a variable font for weight and width. */
private fun archivo(weight: Int) = Font(
    R.font.archivo,
    weight = FontWeight(weight),
    variationSettings = FontVariation.Settings(FontVariation.weight(weight)),
)

val Archivo = FontFamily(archivo(400), archivo(500), archivo(600), archivo(700), archivo(800))

private val Type = Typography(
    displaySmall = TextStyle(fontFamily = Archivo, fontWeight = FontWeight.W800, fontSize = 34.sp, lineHeight = 36.sp, letterSpacing = (-1.0).sp),
    headlineSmall = TextStyle(fontFamily = Archivo, fontWeight = FontWeight.W700, fontSize = 24.sp, lineHeight = 28.sp, letterSpacing = (-0.5).sp),
    titleMedium = TextStyle(fontFamily = Archivo, fontWeight = FontWeight.W600, fontSize = 17.sp, lineHeight = 22.sp, letterSpacing = (-0.2).sp),
    bodyLarge = TextStyle(fontFamily = Archivo, fontWeight = FontWeight.W400, fontSize = 17.sp, lineHeight = 25.sp, letterSpacing = (-0.2).sp),
    bodyMedium = TextStyle(fontFamily = Archivo, fontWeight = FontWeight.W400, fontSize = 15.sp, lineHeight = 21.sp),
    labelLarge = TextStyle(fontFamily = Archivo, fontWeight = FontWeight.W600, fontSize = 15.sp, lineHeight = 20.sp),
    labelSmall = TextStyle(fontFamily = Archivo, fontWeight = FontWeight.W600, fontSize = 11.sp, lineHeight = 14.sp, letterSpacing = 0.9.sp),
)

@Composable
fun MaceTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val c = if (dark) Dark else Light
    val scheme = if (dark) {
        darkColorScheme(
            primary = c.accent, onPrimary = c.accentInk, primaryContainer = c.accentSoft,
            background = c.chat, onBackground = c.text, surface = c.surface, onSurface = c.text,
            surfaceVariant = c.soft, onSurfaceVariant = c.muted, outline = c.line,
            error = c.errorText, errorContainer = c.errorBg,
        )
    } else {
        lightColorScheme(
            primary = c.accent, onPrimary = c.accentInk, primaryContainer = c.accentSoft,
            background = c.chat, onBackground = c.text, surface = c.surface, onSurface = c.text,
            surfaceVariant = c.soft, onSurfaceVariant = c.muted, outline = c.line,
            error = c.errorText, errorContainer = c.errorBg,
        )
    }
    androidx.compose.runtime.CompositionLocalProvider(LocalMaceColors provides c) {
        MaterialTheme(colorScheme = scheme, typography = Type, content = content)
    }
}

object Mace {
    val colors: MaceColors
        @Composable get() = LocalMaceColors.current
}
