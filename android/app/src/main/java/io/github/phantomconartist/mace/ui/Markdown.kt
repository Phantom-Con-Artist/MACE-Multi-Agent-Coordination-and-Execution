package io.github.phantomconartist.mace.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

// Agents write Markdown. This renders the parts they use in chat: fenced
// code, headings, lists, quotes, and inline code, bold, and italics. Code
// keeps its lines and scrolls sideways instead of wrapping.

private sealed interface Block {
    data class Text(val text: String) : Block
    data class Code(val code: String, val language: String) : Block
    data class Heading(val text: String) : Block
    data class Item(val marker: String, val text: String) : Block
    data class Quote(val text: String) : Block
}

private val bullet = Regex("""^\s*[-*+]\s+(.*)""")
private val numbered = Regex("""^\s*(\d+)[.)]\s+(.*)""")

private fun blocks(source: String): List<Block> {
    val out = mutableListOf<Block>()
    val para = StringBuilder()
    fun flush() {
        if (para.isNotBlank()) out += Block.Text(para.toString().trim())
        para.clear()
    }
    val lines = source.lines()
    var i = 0
    while (i < lines.size) {
        val line = lines[i]
        when {
            line.trimStart().startsWith("```") -> {
                flush()
                val language = line.trimStart().removePrefix("```").trim()
                val code = StringBuilder()
                i++
                while (i < lines.size && !lines[i].trimStart().startsWith("```")) {
                    code.appendLine(lines[i]); i++
                }
                out += Block.Code(code.toString().trimEnd('\n'), language)
            }
            line.startsWith("#") -> { flush(); out += Block.Heading(line.trimStart('#').trim()) }
            bullet.matches(line) -> { flush(); out += Block.Item("•", bullet.find(line)!!.groupValues[1]) }
            numbered.matches(line) -> { flush(); numbered.find(line)!!.groupValues.let { out += Block.Item(it[1] + ".", it[2]) } }
            line.startsWith(">") -> { flush(); out += Block.Quote(line.removePrefix(">").trim()) }
            line.isBlank() -> flush()
            else -> { if (para.isNotEmpty()) para.append(' '); para.append(line.trim()) }
        }
        i++
    }
    flush()
    return out
}

private val inline = Regex("""`([^`]+)`|\*\*([^*]+)\*\*|__([^_]+)__|(?<![*\w])\*([^*\s][^*]*)\*(?!\w)|(?<![_\w])_([^_\s][^_]*)_(?!\w)""")

private fun styled(text: String, codeBg: Color): AnnotatedString = buildAnnotatedString {
    var at = 0
    for (m in inline.findAll(text)) {
        append(text.substring(at, m.range.first))
        val (code, bold, bold2, italic, italic2) = m.destructured
        when {
            code.isNotEmpty() -> withStyle(SpanStyle(fontFamily = FontFamily.Monospace, background = codeBg, fontSize = 14.sp)) { append(code) }
            bold.isNotEmpty() || bold2.isNotEmpty() -> withStyle(SpanStyle(fontWeight = FontWeight.W700)) { append(bold + bold2) }
            else -> withStyle(SpanStyle(fontStyle = FontStyle.Italic)) { append(italic + italic2) }
        }
        at = m.range.last + 1
    }
    append(text.substring(at))
}

@Composable
fun Markdown(text: String, color: Color, modifier: Modifier = Modifier) {
    val parsed = remember(text) { blocks(text) }
    val codeBg = MaterialTheme.colorScheme.surfaceContainerHighest
    SelectionContainer(modifier) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            parsed.forEach { b ->
                when (b) {
                    is Block.Text -> Text(styled(b.text, codeBg), style = MaterialTheme.typography.bodyLarge, color = color)
                    is Block.Heading -> Text(styled(b.text, codeBg), style = MaterialTheme.typography.titleMedium, color = color)
                    is Block.Quote -> Text(
                        styled(b.text, codeBg),
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(start = 4.dp).background(codeBg, RoundedCornerShape(6.dp)).padding(horizontal = 10.dp, vertical = 4.dp),
                    )
                    is Block.Item -> Row {
                        Text(b.marker, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(end = 8.dp))
                        Text(styled(b.text, codeBg), style = MaterialTheme.typography.bodyLarge, color = color)
                    }
                    is Block.Code -> Text(
                        b.code,
                        fontFamily = FontFamily.Monospace,
                        fontSize = 13.sp,
                        lineHeight = 19.sp,
                        color = MaterialTheme.colorScheme.onSurface,
                        softWrap = false,
                        modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(codeBg)
                            .horizontalScroll(rememberScrollState()).padding(12.dp),
                    )
                }
            }
        }
    }
}
