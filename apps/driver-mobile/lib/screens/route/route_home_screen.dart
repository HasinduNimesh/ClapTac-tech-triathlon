import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';

import '../../data/driver_models.dart';
import '../../theme/assets.dart';
import '../../theme/tokens.dart';
import '../../widgets/driver_shell.dart';
import '../../widgets/note_banner.dart';
import '../../widgets/svg_icon.dart';
import 'route_assets.dart';
import 'route_widgets.dart';

/// Driver home: vehicle, progress, next stop and today's route.
class RouteHomeScreen extends StatefulWidget {
  const RouteHomeScreen({
    super.key,
    required this.trip,
    required this.onViewStop,
    required this.onReportProblem,
    this.onTabSelected,
  });

  final TripInfo trip;
  final VoidCallback onViewStop;
  final VoidCallback onReportProblem;
  final ValueChanged<DriverTab>? onTabSelected;

  @override
  State<RouteHomeScreen> createState() => _RouteHomeScreenState();
}

class _RouteHomeScreenState extends State<RouteHomeScreen> {
  bool _showOrderHint = true;

  @override
  Widget build(BuildContext context) {
    final trip = widget.trip;
    final next = trip.nextStop;
    final black =
        AppText.of(13, FontWeight.w400, color: Colors.black, height: 19 / 13);

    return DriverShell(
      tab: DriverTab.route,
      onTabSelected: widget.onTabSelected,
      bodyPadding: const EdgeInsets.fromLTRB(
          AppSpace.s24, 21, AppSpace.s24, AppSpace.s24),
      heroTrailing: _ReportProblemPill(onTap: widget.onReportProblem),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const NoteBanner(
            text: 'Use only when safely parked.',
            tone: NoteTone.danger,
            icon: SvgIcon(AppAssets.shieldRed, size: 20),
          ),
          const SizedBox(height: 15),
          _VehicleCard(trip: trip, textStyle: black),
          const SizedBox(height: 13),
          _Progress(completed: trip.completedStops, total: trip.stops.length),
          const SizedBox(height: 17),
          if (next != null)
            _NextStopCard(
                stop: next, onViewStop: widget.onViewStop, textStyle: black)
          else
            const NoteBanner(
                text: 'All stops are completed.', tone: NoteTone.success),
          const SizedBox(height: 17),
          Semantics(
              header: true,
              child: Text('Today’s route',
                  style: AppText.of(13, FontWeight.w700,
                      color: Colors.black, height: 19 / 13))),
          const SizedBox(height: 17),
          _RouteList(
              stops: trip.stops,
              completed: trip.completedStops,
              textStyle: black),
          if (_showOrderHint) ...[
            const SizedBox(height: 17),
            NoteBanner(
              text: 'Follow the planned stop order.',
              icon: const SvgIcon(AppAssets.infoCircle, size: 24),
              onDismiss: () => setState(() => _showOrderHint = false),
            ),
          ],
        ],
      ),
    );
  }
}

class _ReportProblemPill extends StatelessWidget {
  const _ReportProblemPill({required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      label: 'Report a problem',
      excludeSemantics: true,
      child: Material(
        color: Colors.white,
        shape: const StadiumBorder(
            side: BorderSide(color: AppColors.dangerOutline)),
        child: InkWell(
          customBorder: const StadiumBorder(),
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
            child: Text(
              '⚠ Report a problem',
              style: AppText.of(12, FontWeight.w600,
                  color: AppColors.dangerOutline),
            ),
          ),
        ),
      ),
    );
  }
}

class _OutlinedCard extends StatelessWidget {
  const _OutlinedCard(
      {required this.fill,
      required this.border,
      required this.child,
      this.height});

  final Color fill;
  final Color border;
  final double? height;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: height,
      width: double.infinity,
      decoration: BoxDecoration(
        color: fill,
        border: Border.all(color: border),
        borderRadius: BorderRadius.circular(AppRadius.r8),
      ),
      child: child,
    );
  }
}

class _VehicleCard extends StatelessWidget {
  const _VehicleCard({required this.trip, required this.textStyle});

  final TripInfo trip;
  final TextStyle textStyle;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      container: true,
      label:
          'Vehicle ${trip.vehicleLabel}, trip ${trip.tripRef}, ${trip.depot}, ${trip.window}',
      child: ExcludeSemantics(
        child: _OutlinedCard(
          height: 104,
          fill: RouteColors.vehicleCardFill,
          border: RouteColors.vehicleCardBorder,
          child: Stack(
            children: [
              Positioned(
                  left: 12,
                  top: 14,
                  width: 70,
                  height: 70,
                  child: Image.asset(AppAssets.truck, fit: BoxFit.cover)),
              Positioned(
                left: 94,
                top: 13,
                right: 12,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(trip.vehicleLabel,
                        style: textStyle.copyWith(fontWeight: FontWeight.w700)),
                    const SizedBox(height: 4),
                    Text('Trip ${trip.tripRef}', style: textStyle),
                    Text(trip.depot, style: textStyle),
                    Text(trip.window, style: textStyle),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _Progress extends StatelessWidget {
  const _Progress({required this.completed, required this.total});

  final int completed;
  final int total;

  @override
  Widget build(BuildContext context) {
    final fraction = total == 0 ? 0.0 : (completed / total).clamp(0.0, 1.0);
    return Semantics(
      label: '$completed of $total stops completed',
      value: '${(fraction * 100).round()} percent',
      child: ExcludeSemantics(
        child: SizedBox(
          height: 43,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('$completed of $total stops completed',
                  style: AppText.of(14, FontWeight.w400)),
              const SizedBox(height: 8),
              LayoutBuilder(
                builder: (context, constraints) {
                  // The design always shows a small stub of progress.
                  final fill = (constraints.maxWidth * fraction)
                      .clamp(13.0, constraints.maxWidth);
                  return Stack(
                    children: [
                      Container(
                        height: 6,
                        decoration: BoxDecoration(
                            color: AppColors.border,
                            borderRadius: BorderRadius.circular(3)),
                      ),
                      Container(
                        height: 6,
                        width: fill,
                        decoration: BoxDecoration(
                            color: AppColors.primary,
                            borderRadius: BorderRadius.circular(3)),
                      ),
                    ],
                  );
                },
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _NextStopCard extends StatelessWidget {
  const _NextStopCard(
      {required this.stop, required this.onViewStop, required this.textStyle});

  final StopInfo stop;
  final VoidCallback onViewStop;
  final TextStyle textStyle;

  @override
  Widget build(BuildContext context) {
    return _OutlinedCard(
      height: 145,
      fill: RouteColors.nextStopFill,
      border: RouteColors.nextStopBorder,
      child: Stack(
        children: [
          Positioned(
              left: 8,
              top: 7,
              width: 70,
              height: 67,
              child: ExcludeSemantics(
                  child: Image.asset(AppAssets.mapPin, fit: BoxFit.cover))),
          Positioned(
            left: 76,
            top: 3,
            right: 12,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Next Stop',
                    style: textStyle.copyWith(
                        fontWeight: FontWeight.w700,
                        color: RouteColors.nextStopLabel)),
                const SizedBox(height: 2),
                Text(stop.label,
                    style: textStyle.copyWith(fontWeight: FontWeight.w700)),
                const SizedBox(height: 0),
                Text(stop.window, style: textStyle),
                Text(stop.unitsText, style: textStyle),
              ],
            ),
          ),
          Positioned(
            left: 8,
            right: 8,
            top: 86,
            height: 48,
            child: Semantics(
              button: true,
              label: 'View Stop ${stop.name}',
              excludeSemantics: true,
              child: Material(
                color: AppColors.strongBlue,
                borderRadius: BorderRadius.circular(10),
                child: InkWell(
                  borderRadius: BorderRadius.circular(10),
                  onTap: onViewStop,
                  child: Stack(
                    alignment: Alignment.center,
                    children: [
                      Text('View Stop',
                          style: AppText.of(16, FontWeight.w500,
                              color: Colors.white, height: 23 / 16)),
                      const Positioned(
                          right: 17,
                          child: SvgIcon(AppAssets.chevronRight, size: 24)),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _RouteList extends StatelessWidget {
  const _RouteList(
      {required this.stops, required this.completed, required this.textStyle});

  final List<StopInfo> stops;
  final int completed;
  final TextStyle textStyle;

  @override
  Widget build(BuildContext context) {
    return _OutlinedCard(
      fill: Colors.white,
      border: RouteColors.plainCardBorder,
      child: Padding(
        padding: const EdgeInsets.only(top: 3, bottom: 7),
        child: Column(
          children: [
            for (var i = 0; i < stops.length; i++) ...[
              _RouteRow(
                stop: stops[i],
                state: i < completed
                    ? _StepState.done
                    : (i == completed
                        ? _StepState.current
                        : _StepState.upcoming),
                textStyle: textStyle,
              ),
              if (i < stops.length - 1)
                Padding(
                  padding: const EdgeInsets.only(left: 51, right: 28),
                  child: SvgPicture.asset(RouteAssets.divider,
                      height: 1, width: double.infinity, fit: BoxFit.fill),
                ),
            ],
          ],
        ),
      ),
    );
  }
}

enum _StepState { done, current, upcoming }

class _RouteRow extends StatelessWidget {
  const _RouteRow(
      {required this.stop, required this.state, required this.textStyle});

  final StopInfo stop;
  final _StepState state;
  final TextStyle textStyle;

  @override
  Widget build(BuildContext context) {
    final status = switch (state) {
      _StepState.done => 'completed',
      _StepState.current => 'next stop',
      _StepState.upcoming => 'upcoming',
    };
    return Semantics(
      container: true,
      label: 'Stop ${stop.sequence}, ${stop.name}, ${stop.window}, $status',
      child: ExcludeSemantics(
        child: SizedBox(
          height: 48,
          child: Row(
            children: [
              const SizedBox(width: 15),
              SizedBox(
                  width: 27,
                  height: 27,
                  child: Center(
                      child: _StepBadge(number: stop.sequence, state: state))),
              const SizedBox(width: 10),
              Expanded(
                  child: Text(stop.name,
                      style: textStyle.copyWith(fontWeight: FontWeight.w700))),
              Text(stop.window, style: textStyle),
              const SizedBox(width: 28),
            ],
          ),
        ),
      ),
    );
  }
}

class _StepBadge extends StatelessWidget {
  const _StepBadge({required this.number, required this.state});

  final int number;
  final _StepState state;

  @override
  Widget build(BuildContext context) {
    if (state == _StepState.done) {
      return const OverflowBox(
        minWidth: 41,
        maxWidth: 41,
        minHeight: 41,
        maxHeight: 41,
        child: Stack(
          alignment: Alignment.center,
          children: [
            SvgIcon(RouteAssets.stepDoneBackground),
            SvgIcon(RouteAssets.stepDoneCheck, size: 18),
          ],
        ),
      );
    }
    final current = state == _StepState.current;
    return Stack(
      alignment: Alignment.center,
      children: [
        SvgIcon(current ? AppAssets.routeStep1 : AppAssets.routeStep2),
        Text(
          '$number',
          style: AppText.of(13, FontWeight.w700,
              color: current ? Colors.white : Colors.black, height: 19 / 13),
        ),
      ],
    );
  }
}
