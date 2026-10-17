import { atom } from "jotai"
import { atomWithStorage } from "jotai/utils"

export type TourStep = "welcome" | "models" | "completed"

export interface TourState {
  currentStep: TourStep
  isActive: boolean
}

const STORAGE_KEY = "mole-tour-state"

const DEFAULT_TOUR_STATE: TourState = {
  currentStep: "welcome",
  isActive: true,
}

// TOUR_STEP_ORDER is the single order both the guide and the step actions walk:
// the last entry is the terminal step the guide renders nothing for. The tour
// only points at controls that exist: the gateway runs inside this process, so
// there is no start button to teach.
export const TOUR_STEP_ORDER: TourStep[] = ["welcome", "models", "completed"]

export const tourAtom = atomWithStorage<TourState>(
  STORAGE_KEY,
  DEFAULT_TOUR_STATE,
)

export const tourIsActiveAtom = atom(
  (get) => get(tourAtom).isActive,
  (get, set, isActive: boolean) => {
    set(tourAtom, { ...get(tourAtom), isActive })
  },
)

export const tourCurrentStepAtom = atom(
  (get) => get(tourAtom).currentStep,
  (get, set, step: TourStep) => {
    set(tourAtom, { ...get(tourAtom), currentStep: step })
  },
)

export function useTourActions() {
  const goToNextStep = (currentStep: TourStep): TourStep => {
    const currentIndex = TOUR_STEP_ORDER.indexOf(currentStep)
    if (currentIndex < TOUR_STEP_ORDER.length - 1) {
      return TOUR_STEP_ORDER[currentIndex + 1]
    }
    return "completed"
  }

  const goToPrevStep = (currentStep: TourStep): TourStep => {
    const currentIndex = TOUR_STEP_ORDER.indexOf(currentStep)
    if (currentIndex > 0) {
      return TOUR_STEP_ORDER[currentIndex - 1]
    }
    return currentStep
  }

  return { goToNextStep, goToPrevStep }
}
