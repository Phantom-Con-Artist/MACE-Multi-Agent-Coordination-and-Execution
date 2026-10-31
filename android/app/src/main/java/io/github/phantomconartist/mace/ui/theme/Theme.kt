package io.github.phantomconartist.mace.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

// MACE for Android lays out like the messengers people already know, in
// MACE's own palette from the logo: signal red, warm ink, and paper. The
// colors are fixed, not taken from the wallpaper, so MACE always looks
// like MACE.

/** Messenger colors Material's roles do not cover. */
@Immutable
data class MaceColors(
    /** Solid brand red for the new-chat button, send, and unread counts. */
    val signal: Color,
    val onSignal: Color,
    val chat: Color,
    val pattern: Color,
    val outgoing: Color,
    val onOutgoing: Color,
    val incoming: Color,
    val onIncoming: Color,
    /** The dot on an agent that can answer now. */
    val online: Color,
    /** The dot on an agent that is down: its machine is offline or it is stopped. */
    val down: Color,
    /** Two ticks once an agent read the message. */
    val read: Color,
    /** Sender names in groups, one per agent. */
    val names: List<Color>,
)

private val DarkMace = MaceColors(
    signal = Color(0xFFD1170E),
    onSignal = Color(0xFFFFF6F2),
    chat = Color(0xFF0E0B0B),
    pattern = Color(0x0BFFFFFF),
    outgoing = Color(0xFF4A1511),
    onOutgoing = Color(0xFFFDEEEA),
    incoming = Color(0xFF221B1B),
    onIncoming = Color(0xFFF4EEEA),
    online = Color(0xFF5FD38A),
    down = Color(0xFFFF5449),
    read = Color(0xFF7CC4FF),
    names = listOf(Color(0xFFFF8A80), Color(0xFFFFB86B), Color(0xFFC3A6FF), Color(0xFF7CC4FF), Color(0xFF9BE38A), Color(0xFFF48FB1)),
)

private val LightMace = MaceColors(
    signal = Color(0xFFA20000),
    onSignal = Color(0xFFFFF8F4),
    chat = Color(0xFFEFE7DD),
    pattern = Color(0x0D2B0D0B),
    outgoing = Color(0xFFF6D9D1),
    onOutgoing = Color(0xFF2B0D0B),
    incoming = Color(0xFFFFFDFA),
    onIncoming = Color(0xFF171213),
    online = Color(0xFF1E8E4E),
    down = Color(0xFFD32F2F),
    read = Color(0xFF1E7FD6),
    names = listOf(Color(0xFFB3261E), Color(0xFFB0590B), Color(0xFF6A3FC1), Color(0xFF1565C0), Color(0xFF2E7D32), Color(0xFFAD1457)),
)

private val Dark = darkColorScheme(
    primary = Color(0xFFFF6B5D),
    onPrimary = Color(0xFF2B0605),
    primaryContainer = Color(0xFF3A1714),
    onPrimaryContainer = Color(0xFFFFB4A8),
    secondary = Color(0xFFE7BDB8),
    onSecondary = Color(0xFF2B1614),
    secondaryContainer = Color(0xFF3A1714),
    onSecondaryContainer = Color(0xFFFFDAD4),
    tertiary = Color(0xFF7CC4FF),
    background = Color(0xFF120E0E),
    onBackground = Color(0xFFF4EEEA),
    surface = Color(0xFF120E0E),
    onSurface = Color(0xFFF4EEEA),
    surfaceVariant = Color(0xFF241D1D),
    onSurfaceVariant = Color(0xFFA99D99),
    surfaceContainerLowest = Color(0xFF0D0A0A),
    surfaceContainerLow = Color(0xFF181313),
    surfaceContainer = Color(0xFF1D1717),
    surfaceContainerHigh = Color(0xFF241D1D),
    surfaceContainerHighest = Color(0xFF2E2626),
    outline = Color(0xFF4A3F3E),
    outlineVariant = Color(0xFF2F2828),
    error = Color(0xFFFFB4A8),
    onError = Color(0xFF5C0A04),
    errorContainer = Color(0xFF3C1A17),
    onErrorContainer = Color(0xFFFFB4A8),
    inverseSurface = Color(0xFFF4EEEA),
    inverseOnSurface = Color(0xFF1D1717),
)

private val Light = lightColorScheme(
    primary = Color(0xFFA20000),
    onPrimary = Color(0xFFFFF8F4),
    primaryContainer = Color(0xFFF6E2DD),
    onPrimaryContainer = Color(0xFF5C0A04),
    secondary = Color(0xFF775653),
    onSecondary = Color(0xFFFFF8F4),
    secondaryContainer = Color(0xFFF6E2DD),
    onSecondaryContainer = Color(0xFF2B0D0B),
    tertiary = Color(0xFF1E7FD6),
    background = Color(0xFFFFFDFA),
    onBackground = Color(0xFF171213),
    surface = Color(0xFFFFFDFA),
    onSurface = Color(0xFF171213),
    surfaceVariant = Color(0xFFF3EEE8),
    onSurfaceVariant = Color(0xFF6B6160),
    surfaceContainerLowest = Color(0xFFFFFDFA),
    surfaceContainerLow = Color(0xFFF8F4EF),
    surfaceContainer = Color(0xFFF3EEE8),
    surfaceContainerHigh = Color(0xFFEDE6DE),
    surfaceContainerHighest = Color(0xFFE7E0D7),
    outline = Color(0xFFCFC4BA),
    outlineVariant = Color(0xFFE7E0D7),
    error = Color(0xFF87160D),
    onError = Color(0xFFFFF8F4),
    errorContainer = Color(0xFFFBE7E2),
    onErrorContainer = Color(0xFF87160D),
    inverseSurface = Color(0xFF231C1C),
    inverseOnSurface = Color(0xFFF4EEEA),
)

val LocalMace = staticCompositionLocalOf { DarkMace }

/** The messenger colors of the current theme. */
val mace: MaceColors
    @Composable get() = LocalMace.current

@Composable
fun MaceTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    CompositionLocalProvider(LocalMace provides if (dark) DarkMace else LightMace) {
        MaterialTheme(colorScheme = if (dark) Dark else Light, content = content)
    }
}
