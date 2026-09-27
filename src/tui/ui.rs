use crate::tui::app::{App, FocusArea};
use ratatui::{
    layout::{Alignment, Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, Paragraph, Wrap},
    Frame,
};

pub fn draw(f: &mut Frame, app: &mut App) {
    let size = f.area();

    // Vertical layout: Header (3) -> Main (fill) -> Footer (3)
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(3),
            Constraint::Min(12),
            Constraint::Length(3),
        ])
        .split(size);

    draw_header(f, chunks[0]);
    draw_main(f, chunks[1], app);
    draw_footer(f, chunks[2], app);
}

fn draw_header(f: &mut Frame, area: Rect) {
    let header_text = Line::from(vec![
        Span::styled(" agywarp ", Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD)),
        Span::raw("— Process Routing via Cloudflare WARP & Mihomo Core"),
    ]);

    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(Style::default().fg(Color::DarkGray));

    let paragraph = Paragraph::new(header_text)
        .block(block)
        .alignment(Alignment::Left);

    f.render_widget(paragraph, area);
}

fn draw_main(f: &mut Frame, area: Rect, app: &mut App) {
    // Split into Left (Network + Processes) and Right (Console)
    let main_chunks = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Percentage(50),
            Constraint::Percentage(50),
        ])
        .split(area);

    let left_chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(10),
            Constraint::Min(8),
        ])
        .split(main_chunks[0]);

    draw_network_card(f, left_chunks[0], app);
    draw_process_list(f, left_chunks[1], app);
    draw_console(f, main_chunks[1], app);
}

fn draw_network_card(f: &mut Frame, area: Rect, app: &App) {
    let is_focused = app.focus == FocusArea::NetworkCard;
    let border_color = if is_focused { Color::Cyan } else { Color::DarkGray };

    let service_style = if app.service_active {
        Style::default().fg(Color::Green).add_modifier(Modifier::BOLD)
    } else {
        Style::default().fg(Color::Red)
    };
    let service_text = if app.service_active { "ACTIVE (ON)" } else { "INACTIVE (OFF)" };

    let warp_style = if app.warp_status == "CONNECTED" {
        Style::default().fg(Color::Green)
    } else {
        Style::default().fg(Color::Yellow)
    };

    let exit_line = if let Some(ref exit) = app.exit_info {
        format!("{} ({}) | {:?} ping", exit.ip, exit.colo, exit.latency)
    } else if app.service_active {
        "Verifying WARP connection...".to_string()
    } else {
        "---".to_string()
    };

    let text = vec![
        Line::from(vec![
            Span::raw("Routing Service: "),
            Span::styled(service_text, service_style),
            Span::raw("  (Press [Space] to toggle)"),
        ]),
        Line::from(vec![
            Span::raw("Current Node:    "),
            Span::styled(&app.current_node, Style::default().fg(Color::White).add_modifier(Modifier::BOLD)),
        ]),
        Line::from(vec![
            Span::raw("Airport/Source:  "),
            Span::styled(&app.airport_name, Style::default().fg(Color::White)),
        ]),
        Line::from(vec![
            Span::raw("WARP Daemon:     "),
            Span::styled(&app.warp_status, warp_style),
            Span::raw(format!("  (Port: {})", app.warp_port)),
        ]),
        Line::from(vec![
            Span::raw("Protocol Mode:   "),
            Span::styled(app.proxy_mode.as_str().to_uppercase(), Style::default().fg(Color::Magenta).add_modifier(Modifier::BOLD)),
            Span::raw("  (Press [p] to change)"),
        ]),
        Line::from(vec![
            Span::raw("WARP Exit IP:    "),
            Span::styled(exit_line, Style::default().fg(Color::Cyan)),
        ]),
    ];

    let block = Block::default()
        .title(" Network Card ")
        .borders(Borders::ALL)
        .border_style(Style::default().fg(border_color));

    let paragraph = Paragraph::new(text).block(block);
    f.render_widget(paragraph, area);
}

fn draw_process_list(f: &mut Frame, area: Rect, app: &App) {
    let is_focused = app.focus == FocusArea::ProcessList;
    let border_color = if is_focused { Color::Cyan } else { Color::DarkGray };

    let items: Vec<ListItem> = app
        .profiles
        .iter()
        .enumerate()
        .map(|(idx, profile)| {
            let is_selected = idx == app.selected_profile_idx;
            let (status_text, status_color) = if profile.enabled {
                ("[ON] ", Color::Green)
            } else {
                ("[OFF]", Color::DarkGray)
            };

            let prefix = if is_selected { "▶ " } else { "  " };

            let matchers_summary: Vec<String> = profile
                .matchers
                .iter()
                .map(|m| m.pattern.clone())
                .collect();

            let line = Line::from(vec![
                Span::styled(prefix, Style::default().fg(Color::Cyan)),
                Span::styled(status_text, Style::default().fg(status_color).add_modifier(Modifier::BOLD)),
                Span::raw(" "),
                Span::styled(&profile.label, Style::default().fg(Color::White).add_modifier(Modifier::BOLD)),
                Span::raw(" ("),
                Span::styled(matchers_summary.join(", "), Style::default().fg(Color::DarkGray)),
                Span::raw(")"),
            ]);

            ListItem::new(line)
        })
        .collect();

    let block = Block::default()
        .title(" Process Groups ([Space] Toggle, [↑/↓] Select) ")
        .borders(Borders::ALL)
        .border_style(Style::default().fg(border_color));

    let list = List::new(items).block(block);
    f.render_widget(list, area);
}

fn draw_console(f: &mut Frame, area: Rect, app: &App) {
    let is_focused = app.focus == FocusArea::Console;
    let border_color = if is_focused { Color::Cyan } else { Color::DarkGray };

    let logs: Vec<Line> = app
        .logs
        .iter()
        .map(|l| {
            let color = if l.contains("[OK]") {
                Color::Green
            } else if l.contains("[WARN]") {
                Color::Yellow
            } else if l.contains("[ERROR]") {
                Color::Red
            } else {
                Color::Gray
            };
            Line::from(Span::styled(l, Style::default().fg(color)))
        })
        .collect();

    let block = Block::default()
        .title(" Output Console ([↑/↓] Scroll) ")
        .borders(Borders::ALL)
        .border_style(Style::default().fg(border_color));

    let paragraph = Paragraph::new(logs)
        .block(block)
        .wrap(Wrap { trim: false })
        .scroll((app.console_scroll as u16, 0));

    f.render_widget(paragraph, area);
}

fn draw_footer(f: &mut Frame, area: Rect, app: &App) {
    let focus_hint = match app.focus {
        FocusArea::NetworkCard => "Focused: Network Card",
        FocusArea::ProcessList => "Focused: Process Groups",
        FocusArea::Console => "Focused: Output Console",
    };

    let text = Line::from(vec![
        Span::styled(format!(" [{}] ", focus_hint), Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD)),
        Span::raw(" | [Tab] Switch Focus | [Space] Toggle | [p] Protocol | [r] Refresh | [q] Quit"),
    ]);

    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(Style::default().fg(Color::DarkGray));

    let paragraph = Paragraph::new(text)
        .block(block)
        .alignment(Alignment::Center);

    f.render_widget(paragraph, area);
}
